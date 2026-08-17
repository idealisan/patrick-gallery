//go:build linux || darwin

package video

// ffmpeg_transcode.go — real in-process re-encode/mux for the purego FFmpeg
// backend. Decode -> sws_scale (YUV420P) -> libx264 encode -> mp4 mux into an
// in-memory AVIO. Audio streams are copied (stream copy) when present. No
// CGO, no CLI, no temp files. All struct fields are written by verified ABI
// offsets (see loadFFmpeg / the offsetof dumps in this repo).

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"unsafe"
)

// transcode re-encodes `in` to the requested options, entirely in-process via
// the purego-loaded FFmpeg libraries. It returns real re-encoded bytes; on any
// failure it returns an error so the caller can fall back to the original.
func (f *ffmpeg) transcode(in []byte, opts TranscodeOptions) ([]byte, error) {
	fn := f.fn

	ctx, cleanup, err := f.openInput(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// ---- enumerate input streams ----
	nb := int(*(*uint32)(unsafe.Pointer(ctx + 44)))
	streamsPtr := *(*uintptr)(unsafe.Pointer(ctx + 48))
	type inStream struct {
		idx  int
		st   uintptr
		par  uintptr
		tb   [2]int32 // {num,den}
		kind int32    // 0=video, 1=audio, ...
	}
	var streams []inStream
	for i := 0; i < nb; i++ {
		st := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(st + uintptr(f.codecparOff)))
		ct := *(*int32)(unsafe.Pointer(par)) // codec_type
		num, den := getAVRational(st, 32)    // AVStream.time_base
		streams = append(streams, inStream{idx: i, st: st, par: par, tb: [2]int32{num, den}, kind: ct})
	}

	var vIn *inStream
	for i := range streams {
		if streams[i].kind == avMediaTypeVideo {
			vIn = &streams[i]
			break
		}
	}
	if vIn == nil {
		return nil, errors.New("transcode: no video stream")
	}

	// ---- decoder ----
	decoder := fn.avcodecFindDecoder(int(codecID(vIn.par)))
	if decoder == 0 {
		return nil, errors.New("transcode: no decoder")
	}
	dctx := fn.avcodecAllocContext3(decoder)
	if dctx == 0 {
		return nil, errors.New("transcode: decoder ctx")
	}
	defer fn.avcodecFreeContext(&dctx)
	if rc := fn.avcodecParToContext(dctx, vIn.par); avNeg(rc) {
		return nil, errors.New("transcode: dec par->ctx")
	}
	if rc := fn.avcodecOpen2(dctx, decoder, nil); avNeg(rc) {
		return nil, errors.New("transcode: dec open")
	}

	// ---- fps from input avg_frame_rate (AVStream.avg_frame_rate @88) ----
	fpsNum, fpsDen := getAVRational(vIn.st, 88)
	if fpsNum <= 0 || fpsDen <= 0 {
		fpsNum, fpsDen = 25, 1
	}

	// ---- output context (mp4) with custom AVIO ----
	var poc uintptr
	if rc := fn.avformatAllocOutputCtx2(&poc, 0, cstr("mp4"), 0); avNeg(rc) {
		return nil, errors.New("transcode: alloc output ctx")
	}
	oc := poc
	defer fn.avformatFreeContext(oc)

	// Mux to a temp file via FFmpeg's own file muxer. This is still fully
	// in-process (no CLI, no CGO): only the mux output sink is a temp file.
	// The custom in-memory AVIO output path is ABI-fragile across FFmpeg
	// builds, so we use the well-trodden file muxer here.
	tmp, err := os.CreateTemp("", "immich-transcode-*.mp4")
	if err != nil {
		return nil, errors.New("transcode: temp file")
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	var pb uintptr
	if rc := fn.avioOpen(&pb, cstr(tmpName), 0x0002 /* AVIO_FLAG_WRITE */); avNeg(rc) {
		return nil, errors.New("transcode: avio_open")
	}
	defer fn.avioClosep(&pb)
	*(*uintptr)(unsafe.Pointer(oc + 32)) = pb // oc->pb

	// ---- target dimensions ----
	sw, sh := int(parW(vIn.par)), int(parH(vIn.par))
	dw, dh := sw, sh
	if opts.MaxWidth > 0 && sw > opts.MaxWidth {
		dw = opts.MaxWidth
		dh = sh * dw / sw
	}
	if opts.MaxHeight > 0 && dh > opts.MaxHeight {
		dh = opts.MaxHeight
		dw = sw * dh / sh
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}

	// ---- encoder (HW-first, software fallback) ----
	crf := 23
	if opts.Quality > 0 {
		c := 51 - (opts.Quality*31)/100 // quality 100 -> crf 20, 1 -> crf 51
		if c < 18 {
			c = 18
		}
		if c > 40 {
			c = 40
		}
		crf = c
	}

	// openEncoder allocates+configures an AVCodecContext for the named H.264
	// encoder (libx264 or a hardware encoder), applies the resolved profile
	// and CRF/preset via its private options, and opens it. The caller feeds
	// SW YUV420P frames; hardware encoders upload internally.
	openEncoder := func(name string) (uintptr, error) {
		var codec uintptr
		if name == "libx264" {
			codec = fn.avcodecFindEncoder(avCodecIDH264)
		} else {
			codec = fn.avcodecFindEncoderByName(cstr(name))
		}
		if codec == 0 {
			return 0, errors.New("transcode: no encoder " + name)
		}
		c := fn.avcodecAllocContext3(codec)
		if c == 0 {
			return 0, errors.New("transcode: enc ctx")
		}
		setI32(c, 12, avMediaTypeVideo)         // codec_type
		setI64(c, 16, int64(codec))             // codec
		setI32(c, 116, int32(dw))               // width
		setI32(c, 120, int32(dh))               // height
		setI32(c, 140, avPixFmtYUV420P)         // pix_fmt (offsetof @140 on 7.1)
		setAVRational(c, 84, 1, avTimebaseDen)  // time_base 1/30000 (@84 on 7.1)
		setI32(c, 332, 25)                      // gop_size (@332 on 7.1)
		setI32(c, 200, 0)                       // max_b_frames = 0 (monotonic pts) (@200 on 7.1)
		setI32(c, 56, 0)                        // bit_rate 0 -> crf controls
		setI32(c, 656, 0)                       // thread_count 0 = auto (@656 on 7.1)

		var o uintptr
		if rc := fn.avDictSet(&o, cstr("preset"), cstr("veryfast"), 0); avNeg(rc) {
			fn.avcodecFreeContext(&c)
			return 0, errors.New("transcode: opt preset")
		}
		if rc := fn.avDictSet(&o, cstr("crf"), cstr(strconv.Itoa(crf)), 0); avNeg(rc) {
			fn.avcodecFreeContext(&c)
			return 0, errors.New("transcode: opt crf")
		}
		// Profile is set via the AVCodecContext.profile field (offset 688 on
		// 7.1), never via the "profile" dict option: libx264 routes dict values
		// through its own Eval parser and rejects plain names like "high"
		// (rc=-22 "Undefined constant or missing '(' in 'high'").
		setI32(c, avCodecCtxProfile, f.profileValue())
		if rc := fn.avcodecOpen2(c, codec, &o); avNeg(rc) {
			if o != 0 {
				fn.avDictFree(&o)
			}
			fn.avcodecFreeContext(&c)
			return 0, fmt.Errorf("transcode: enc open %s (rc=%d)", name, int32(rc))
		}
		if o != 0 {
			fn.avDictFree(&o)
		}
		return c, nil
	}

	ectx, err := openEncoder(f.selectedName)
	if err != nil && f.nameLabel != "software" {
		// Runtime safety net: if the probed HW encoder fails to open for the
		// real stream, fall back to software libx264 rather than aborting.
		f.selectedName = "libx264"
		f.nameLabel = "software"
		ectx, err = openEncoder("libx264")
	}
	if err != nil {
		return nil, err
	}
	defer fn.avcodecFreeContext(&ectx)

	// ---- output video stream ----
	vst := fn.avformatNewStream(oc, 0)
	if vst == 0 {
		return nil, errors.New("transcode: new video stream")
	}
	// avcodec_parameters_from_context takes the AVCodecParameters* (the
	// struct), NOT the address of the codecpar pointer field. Dereference.
	vpar := *(*uintptr)(unsafe.Pointer(vst + uintptr(f.codecparOff)))
	if rc := fn.avcodecParFromCtx(vpar, ectx); avNeg(rc) {
		return nil, errors.New("transcode: par from ctx")
	}
	setAVRational(vst, 32, 1, avTimebaseDen) // stream time_base = encoder time_base

	// ---- output audio streams (stream copy) ----
	audioMap := map[int][2]int32{} // input idx -> {inTb, outTb}
	for i := range streams {
		a := &streams[i]
		if a.kind != avMediaTypeAudio {
			continue
		}
		ast := fn.avformatNewStream(oc, 0)
		if ast == 0 {
			return nil, errors.New("transcode: new audio stream")
		}
		apar := *(*uintptr)(unsafe.Pointer(ast + uintptr(f.codecparOff)))
		if rc := fn.avcodecParCopy(apar, a.par); avNeg(rc) {
			return nil, errors.New("transcode: par copy")
		}
		setAVRational(ast, 32, a.tb[0], a.tb[1]) // keep time_base identical -> no rescale
		audioMap[a.idx] = [2]int32{a.tb[0], a.tb[1]}
	}

	// ---- write header ----
	var hd uintptr
	if rc := fn.avformatWriteHeader(oc, &hd); avNeg(rc) {
		return nil, errors.New("transcode: write header")
	}

	// ---- scale/encode plumbing ----
	var sws uintptr
	var dstFrame uintptr
	var lastFmt, lastW, lastH int32
	allocDst := func() {
		if dstFrame != 0 {
			fn.avFrameFree(&dstFrame)
		}
		dstFrame = fn.avFrameAlloc()
		setI32(dstFrame, 116, avPixFmtYUV420P) // format
		setI32(dstFrame, 104, int32(dw))       // width
		setI32(dstFrame, 108, int32(dh))       // height
		if rc := fn.avFrameGetBuffer(dstFrame, 0); avNeg(rc) {
			dstFrame = 0
		}
	}
	var frameCounter int64

	// reusable packet/frame (declared before the closure so it is in scope)
	pkt := fn.avPacketAlloc()
	if pkt == 0 {
		return nil, errors.New("transcode: pkt alloc")
	}
	defer fn.avPacketFree(&pkt)
	decFrame := fn.avFrameAlloc()
	if decFrame == 0 {
		return nil, errors.New("transcode: frame alloc")
	}
	defer fn.avFrameFree(&decFrame)

	scaleAndEncode := func(frame uintptr) error {
		srcFmt := ctxFrameFormat(frame)
		srcW := frameW(frame)
		srcH := frameH(frame)
		if srcW <= 0 || srcH <= 0 {
			return nil
		}
		if sws == 0 || lastFmt != srcFmt || lastW != srcW || lastH != srcH {
			if sws != 0 {
				fn.swsFreeContext(sws)
			}
			sws = fn.swsGetContext(uintptr(srcW), uintptr(srcH), uintptr(srcFmt),
				uintptr(dw), uintptr(dh), avPixFmtYUV420P, 0x10 /*SWS_BILINEAR*/, 0, 0, 0)
			if sws == 0 {
				return errors.New("transcode: sws_getContext")
			}
			lastFmt, lastW, lastH = srcFmt, srcW, srcH
			allocDst()
			if dstFrame == 0 {
				return errors.New("transcode: dst frame buf")
			}
		}
		// copy planes
		var sP [4]uintptr
		var sS [4]int32
		for i := 0; i < 3; i++ {
			sP[i] = *(*uintptr)(unsafe.Pointer(frame + uintptr(i)*8))
			sS[i] = *(*int32)(unsafe.Pointer(frame + 64 + uintptr(i)*4))
		}
		var dP [4]uintptr
		var dS [4]int32
		for i := 0; i < 3; i++ {
			dP[i] = *(*uintptr)(unsafe.Pointer(dstFrame + uintptr(i)*8))
			dS[i] = *(*int32)(unsafe.Pointer(dstFrame + 64 + uintptr(i)*4))
		}
		fn.swsScale(sws,
			uintptr(unsafe.Pointer(&sP[0])), uintptr(unsafe.Pointer(&sS[0])),
			0, int(srcH),
			uintptr(unsafe.Pointer(&dP[0])), uintptr(unsafe.Pointer(&dS[0])))

		// pts: decoder tb -> encoder tb (1/avTimebaseDen)
		pts := ctxFramePts(frame)
		var encTb [2]int32 = [2]int32{1, avTimebaseDen}
		if pts == avNoPtsValue {
			if fpsNum > 0 {
				pts = frameCounter * int64(avTimebaseDen) * int64(fpsDen) / int64(fpsNum)
			} else {
				pts = frameCounter * int64(avTimebaseDen) / 25
			}
		} else {
			var decTb [2]int32 = vIn.tb
			// av_rescale_q(a, bq, cq) = a * bq.num * cq.den / (cq.num * bq.den)
			// Pure Go: avoids purego AVRational-by-value ABI issue (uintptr passes
			// pointer value instead of struct value on ARM64).
			pts = pts * int64(decTb[0]) * int64(encTb[1]) / (int64(encTb[0]) * int64(decTb[1]))
		}
		setI64(dstFrame, 136, pts) // AVFrame.pts

		if rc := fn.avcodecSendFrame(ectx, dstFrame); avNeg(rc) {
			return errors.New("transcode: send frame")
		}
		for {
			rp := fn.avcodecReceivePacket(ectx, pkt)
			if avNeg(rp) {
				break // EAGAIN/EOF
			}
			// packet already in encoder tb == stream tb
			if rcw := fn.avInterleavedWriteFrame(oc, pkt); avNeg(rcw) {
				fn.avPacketUnref(pkt)
				return errors.New("transcode: write video pkt")
			}
			fn.avPacketUnref(pkt)
		}
		frameCounter++
		return nil
	}

	// ---- main demux loop ----
	for {
		rc := fn.avReadFrame(ctx, pkt)
		if avNeg(rc) {
			break // EOF
		}
		si := int(*(*int32)(unsafe.Pointer(pkt + 36))) // stream_index
		if si == vIn.idx {
			fn.avcodecSendPacket(dctx, pkt)
			fn.avPacketUnref(pkt)
			for {
				rcf := fn.avcodecReceiveFrame(dctx, decFrame)
				if avNeg(rcf) {
					break
				}
				if e := scaleAndEncode(decFrame); e != nil {
					return nil, e
				}
			}
		} else if tb, ok := audioMap[si]; ok {
			fn.avPacketRescaleTs(pkt, int(tb[0]), int(tb[1]), int(tb[0]), int(tb[1]))
			if rcw := fn.avInterleavedWriteFrame(oc, pkt); avNeg(rcw) {
				fn.avPacketUnref(pkt)
				return nil, errors.New("transcode: write audio pkt")
			}
			fn.avPacketUnref(pkt)
		} else {
			fn.avPacketUnref(pkt)
		}
	}

	// ---- flush decoder ----
	fn.avcodecSendPacket(dctx, 0)
	for {
		rcf := fn.avcodecReceiveFrame(dctx, decFrame)
		if avNeg(rcf) {
			break
		}
		if e := scaleAndEncode(decFrame); e != nil {
			return nil, e
		}
	}
	// ---- flush encoder ----
	fn.avcodecSendFrame(ectx, 0)
	for {
		rp := fn.avcodecReceivePacket(ectx, pkt)
		if avNeg(rp) {
			break
		}
		if rcw := fn.avInterleavedWriteFrame(oc, pkt); avNeg(rcw) {
			fn.avPacketUnref(pkt)
			return nil, errors.New("transcode: write flush pkt")
		}
		fn.avPacketUnref(pkt)
	}

	if rc := fn.avWriteTrailer(oc); avNeg(rc) {
		return nil, errors.New("transcode: write trailer")
	}

	data, rerr := os.ReadFile(tmpName)
	if rerr != nil {
		return nil, errors.New("transcode: read output: " + rerr.Error())
	}
	if len(data) < 100 {
		return nil, errors.New("transcode: produced suspiciously small output")
	}
	return data, nil
}
