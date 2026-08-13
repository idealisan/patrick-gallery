//go:build linux || darwin

package video

import (
	"errors"
	"image"
	"unsafe"
)

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// videoStream returns the first video stream index and its AVCodecParameters.
func (f *ffmpeg) videoStream(ctx uintptr) (int, uintptr, error) {
	nb := *(*uint32)(unsafe.Pointer(ctx + 44)) // AVFormatContext.nb_streams
	streamsPtr := *(*uintptr)(unsafe.Pointer(ctx + 48)) // AVFormatContext.streams
	for i := uint32(0); i < nb; i++ {
		streamPtr := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(streamPtr + uintptr(f.codecparOff)))
		if *(*int32)(unsafe.Pointer(par)) == avMediaTypeVideo { // codec_type
			return int(i), par, nil
		}
	}
	return -1, 0, errors.New("no video stream")
}

func codecID(par uintptr) int32 { return *(*int32)(unsafe.Pointer(par + 4)) }

// decodeFrame iterates the container, decodes video packets with the decoder
// context cctx, and invokes cb for each decoded video AVFrame. It stops early
// (without consuming the whole file) when cb returns errStopDecode, which the
// caller uses to grab just the first frame for a thumbnail.
func (f *ffmpeg) decodeFrame(cctx, fmtCtx uintptr, vidx int, cb func(frame uintptr) error) error {
	fn := f.fn
	frame := fn.avFrameAlloc()
	pkt := fn.avPacketAlloc()
	if frame == 0 || pkt == 0 {
		return errors.New("alloc failed")
	}
	defer fn.avFrameFree(&frame)

	// Feed packets until we have what we need, draining frames as they come.
	var pkts int
	for {
		if pkts > 100000 {
			return errors.New("decodeFrame: too many packets (loop?)")
		}
		rc := fn.avReadFrame(fmtCtx, pkt)
		eof := avNeg(rc)
		if !eof && int(*(*int32)(unsafe.Pointer(pkt+36))) != vidx { // stream_index
			fn.avPacketUnref(pkt)
			continue
		}
		// Send the packet (or flush the decoder with NULL at EOF).
		if !eof {
			pkts++
			fn.avcodecSendPacket(cctx, pkt)
			fn.avPacketUnref(pkt)
		} else {
			fn.avcodecSendPacket(cctx, 0) // flush
		}
		// Drain all frames produced so far.
		for {
			rcf := fn.avcodecReceiveFrame(cctx, frame)
			if avNeg(rcf) {
				break // EAGAIN or EOF
			}
			if err := cb(frame); err != nil {
				if err == errStopDecode {
					return nil // early exit OK
				}
				return err
			}
		}
		if eof {
			break
		}
	}
	return nil
}

// Thumbnail decodes the first video frame and returns an encoded image.
func (f *ffmpeg) Thumbnail(in []byte, opts ThumbnailOptions) ([]byte, error) {
	ctx, cleanup, err := f.openInputMemory(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	vidx, par, err := f.videoStream(ctx)
	if err != nil {
		return nil, err
	}
	decoder := f.fn.avcodecFindDecoder(int(codecID(par)))
	if decoder == 0 {
		return nil, errors.New("no decoder for codec")
	}
	cctx := f.fn.avcodecAllocContext3(decoder)
	if cctx == 0 {
		return nil, errors.New("avcodec_alloc_context3 failed")
	}
	if rc := f.fn.avcodecParToContext(cctx, par); avNeg(rc) {
		return nil, errors.New("parameters_to_context failed")
	}
	if rc := f.fn.avcodecOpen2(cctx, decoder, nil); avNeg(rc) {
		return nil, errors.New("avcodec_open2 failed")
	}

	edge := opts.MaxEdge
	if edge <= 0 {
		edge = 320
	}
	format := opts.Format
	if format == "" {
		format = "jpeg"
	}

	var out []byte
	err = f.decodeFrame(cctx, ctx, vidx, func(frame uintptr) error {
		w := int(frameW(frame))
		h := int(frameH(frame))
		if w == 0 || h == 0 {
			return nil // skip until a real frame
		}
		scale := float64(edge) / float64(maxInt(w, h))
		dw, dh := w, h
		if w >= h {
			dh = int(float64(h) * scale)
		} else {
			dw = int(float64(w) * scale)
		}
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
		sws := f.fn.swsGetContext(uintptr(w), uintptr(h), avPixFmtYUV420P, uintptr(dw), uintptr(dh), avPixFmtRGBA,
			0x10 /*SWS_BILINEAR*/, 0, 0, 0)
		if sws == 0 {
			return errors.New("sws_getContext failed")
		}
		defer f.fn.swsFreeContext(sws)
		rgbaBuf := f.fn.avMalloc(uintptr(dw * dh * 4))
		if rgbaBuf == 0 {
			return errors.New("av_malloc failed")
		}
		defer f.fn.avFree(rgbaBuf)

		var srcPlanes [4]uintptr
		var srcStrides [4]int32
		srcPlanes[0] = frameData0(frame)
		srcStrides[0] = frameStride0(frame)
		var dstPlanes [4]uintptr
		var dstStrides [4]int32
		dstPlanes[0] = rgbaBuf
		dstStrides[0] = int32(dw * 4)

		f.fn.swsScale(sws,
			uintptr(unsafe.Pointer(&srcPlanes[0])), uintptr(unsafe.Pointer(&srcStrides[0])),
			0, h,
			uintptr(unsafe.Pointer(&dstPlanes[0])), uintptr(unsafe.Pointer(&dstStrides[0])))

		src := unsafe.Slice((*byte)(unsafe.Pointer(rgbaBuf)), dw*dh*4)
		img := image.NewRGBA(image.Rect(0, 0, dw, dh))
		for y := 0; y < dh; y++ {
			copy(img.Pix[y*img.Stride:(y+1)*img.Stride], src[y*dw*4:(y+1)*dw*4])
		}
		enc, e := EncodeImage(img, format)
		if e != nil {
			return e
		}
		out = enc
		return errStopDecode // first frame is enough
	})
	if err != nil && err != errStopDecode {
		return nil, err
	}
	if out == nil {
		return nil, errors.New("no frame decoded")
	}
	return out, nil
}

var errStopDecode = errors.New("stop")

// Probe returns container/stream metadata.
func (f *ffmpeg) Probe(in []byte) (*Metadata, error) {
	ctx, cleanup, err := f.openInputMemory(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	m := &Metadata{Raw: in}
	if vidx, par, e := f.videoStream(ctx); e == nil {
		m.HasVideo = true
		m.Width = int(parW(par))
		m.Height = int(parH(par))
		m.VideoCodec = codecName(int(codecID(par)))
		_ = vidx
	}
	// first audio stream (codec_type==1)
	nb := *(*uint32)(unsafe.Pointer(ctx + 44)) // AVFormatContext.nb_streams
	streamsPtr := *(*uintptr)(unsafe.Pointer(ctx + 48)) // AVFormatContext.streams
	for i := uint32(0); i < nb; i++ {
		streamPtr := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(streamPtr + uintptr(f.codecparOff)))
		if *(*int32)(unsafe.Pointer(par)) == 1 { // AVMEDIA_TYPE_AUDIO
			m.HasAudio = true
			m.AudioCodec = codecName(int(codecID(par)))
			break
		}
	}
	return m, nil
}

// Transcode re-encodes in to the requested options. The purego re-encode
// path (decode -> sws_scale -> libavcodec encode -> mux) is implemented in
// ffmpeg_transcode.go. This entrypoint delegates to it and lets any failure
// propagate so the caller can fall back to the original file.
func (f *ffmpeg) Transcode(in []byte, opts TranscodeOptions) ([]byte, error) {
	return f.transcode(in, opts)
}

// codecName maps a FFmpeg codec_id to a short label.
func codecName(id int) string {
	switch id {
	case 27: // H264
		return "h264"
	case 28: // H265/HEVC
		return "hevc"
	case 52: // MJPEG
		return "mjpeg"
	case 13: // MPEG4
		return "mpeg4"
	case 2: // DV
		return "dvvideo"
	case 86076: // VP9
		return "vp9"
	case 167: // AV1
		return "av1"
	case 173: // HEVC
		return "hevc"
	case 86018: // AAC
		return "aac"
	case 86028: // MP3
		return "mp3"
	case 8: // PCM
		return "pcm"
	}
	return "unknown"
}
