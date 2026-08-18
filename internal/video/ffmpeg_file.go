//go:build linux || darwin

package video

// ffmpeg_file.go — file-to-file transcoding and probing for the purego FFmpeg
// backend. These methods read from disk files via FFmpeg's streaming I/O
// (avformat_open_input) instead of loading entire files into Go memory.

import (
	"errors"
	"strconv"
	"unsafe"
)

// ProbeFile reads metadata from a file on disk without loading the entire file
// into memory. FFmpeg's avformat_open_input does streaming I/O, reading only
// the header and stream info.
func (f *ffmpeg) ProbeFile(path string) (*Metadata, error) {
	fn := f.fn

	// Open input file via FFmpeg (streaming I/O, reads only headers).
	var pfmtCtx uintptr
	if rc := fn.avformatOpenInput(&pfmtCtx, cstr(path), 0, nil); avNeg(rc) {
		return nil, errors.New("probe: open " + path)
	}
	defer fn.avformatCloseInput(&pfmtCtx)

	// Read stream info (may read a few packets to get accurate fps/duration).
	if rc := fn.avformatFindStreamInfo(pfmtCtx, nil); avNeg(rc) {
		return nil, errors.New("probe: find stream info")
	}

	return f.probeFromCtx(pfmtCtx), nil
}

// probeFromCtx extracts metadata from an already-opened AVFormatContext.
func (f *ffmpeg) probeFromCtx(ctx uintptr) *Metadata {
	meta := &Metadata{}

	// Extract overall bitrate from AVFormatContext.bit_rate (offset 112).
	meta.Bitrate = int64(*(*int64)(unsafe.Pointer(ctx + 112)))

	nb := int(*(*uint32)(unsafe.Pointer(ctx + 44)))
	streamsPtr := *(*uintptr)(unsafe.Pointer(ctx + 48))

	for i := 0; i < nb; i++ {
		st := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(st + uintptr(f.codecparOff)))
		ct := *(*int32)(unsafe.Pointer(par)) // codec_type

		if ct == avMediaTypeVideo && !meta.HasVideo {
			meta.HasVideo = true
			meta.Width = int(parW(par))
			meta.Height = int(parH(par))
			meta.DurationSec = f.streamDurationSec(ctx, i)
			meta.VideoCodec = codecName(int(codecID(par)))
		} else if ct == avMediaTypeAudio && !meta.HasAudio {
			meta.HasAudio = true
			meta.AudioCodec = codecName(int(codecID(par)))
		}
	}

	return meta
}

// TranscodeToFile re-encodes srcPath and writes the result to dstPath.
// This is a streaming file-to-file operation: FFmpeg reads from srcPath via
// streaming I/O and writes to dstPath directly. Hardware encoders are tried
// first (VideoToolbox -> NVENC -> QSV -> AMF -> software).
func (f *ffmpeg) TranscodeToFile(srcPath, dstPath string, opts TranscodeOptions) error {
	fn := f.fn

	// ---- open input file ----
	var ifmtCtx uintptr
	if rc := fn.avformatOpenInput(&ifmtCtx, cstr(srcPath), 0, nil); avNeg(rc) {
		return errors.New("transcode: open " + srcPath)
	}
	defer fn.avformatCloseInput(&ifmtCtx)

	if rc := fn.avformatFindStreamInfo(ifmtCtx, nil); avNeg(rc) {
		return errors.New("transcode: find stream info")
	}

	// ---- enumerate input streams ----
	nb := int(*(*uint32)(unsafe.Pointer(ifmtCtx + 44)))
	streamsPtr := *(*uintptr)(unsafe.Pointer(ifmtCtx + 48))
	type inStream struct {
		idx  int
		st   uintptr
		par  uintptr
		tb   [2]int32
		kind int32
	}
	var streams []inStream
	for i := 0; i < nb; i++ {
		st := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(st + uintptr(f.codecparOff)))
		ct := *(*int32)(unsafe.Pointer(par))
		num, den := getAVRational(st, 32)
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
		return errors.New("transcode: no video stream")
	}

	// ---- open decoder ----
	decoder := fn.avcodecFindDecoder(int(codecID(vIn.par)))
	if decoder == 0 {
		return errors.New("transcode: no decoder")
	}
	dctx := fn.avcodecAllocContext3(decoder)
	if dctx == 0 {
		return errors.New("transcode: decoder ctx")
	}
	defer fn.avcodecFreeContext(&dctx)
	if rc := fn.avcodecParToContext(dctx, vIn.par); avNeg(rc) {
		return errors.New("transcode: dec par->ctx")
	}
	if rc := fn.avcodecOpen2(dctx, decoder, nil); avNeg(rc) {
		return errors.New("transcode: dec open")
	}

	// ---- fps from input avg_frame_rate ----
	fpsNum, fpsDen := getAVRational(vIn.st, 88)
	if fpsNum <= 0 || fpsDen <= 0 {
		fpsNum, fpsDen = 25, 1
	}

	// ---- output context (mp4) ----
	var oc uintptr
	if rc := fn.avformatAllocOutputCtx2(&oc, 0, cstr("mp4"), 0); avNeg(rc) {
		return errors.New("transcode: alloc output ctx")
	}
	defer fn.avformatFreeContext(oc)

	// Open output file via FFmpeg's file muxer.
	var pb uintptr
	if rc := fn.avioOpen(&pb, cstr(dstPath), 0x0002 /* AVIO_FLAG_WRITE */); avNeg(rc) {
		return errors.New("transcode: avio_open dst")
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
		c := 51 - (opts.Quality*31)/100
		if c < 18 {
			c = 18
		}
		if c > 40 {
			c = 40
		}
		crf = c
	}

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
		setI32(c, 12, avMediaTypeVideo)
		setI64(c, 16, int64(codec))
		setI32(c, 116, int32(dw))
		setI32(c, 120, int32(dh))
		setI32(c, 140, avPixFmtYUV420P)
		setAVRational(c, 84, 1, avTimebaseDen)
		setI32(c, 332, 25)
		setI32(c, 200, 0)
		setI32(c, 56, 0)
		setI32(c, 656, 0)

		var o uintptr
		if rc := fn.avDictSet(&o, cstr("preset"), cstr("veryfast"), 0); avNeg(rc) {
			fn.avcodecFreeContext(&c)
			return 0, errors.New("transcode: opt preset")
		}
		if rc := fn.avDictSet(&o, cstr("crf"), cstr(strconv.Itoa(crf)), 0); avNeg(rc) {
			fn.avcodecFreeContext(&c)
			return 0, errors.New("transcode: opt crf")
		}
		setI32(c, avCodecCtxProfile, f.profileValue())
		if rc := fn.avcodecOpen2(c, codec, &o); avNeg(rc) {
			if o != 0 {
				fn.avDictFree(&o)
			}
			fn.avcodecFreeContext(&c)
			return 0, errors.New("transcode: enc open " + name)
		}
		if o != 0 {
			fn.avDictFree(&o)
		}
		return c, nil
	}

	ectx, err := openEncoder(f.selectedName)
	if err != nil && f.nameLabel != "software" {
		f.selectedName = "libx264"
		f.nameLabel = "software"
		ectx, err = openEncoder("libx264")
	}
	if err != nil {
		return err
	}
	defer fn.avcodecFreeContext(&ectx)

	// ---- output video stream ----
	vst := fn.avformatNewStream(oc, 0)
	if vst == 0 {
		return errors.New("transcode: new video stream")
	}
	vpar := *(*uintptr)(unsafe.Pointer(vst + uintptr(f.codecparOff)))
	if rc := fn.avcodecParFromCtx(vpar, ectx); avNeg(rc) {
		return errors.New("transcode: par from ctx")
	}
	setAVRational(vst, 32, 1, avTimebaseDen)

	// ---- output audio streams (stream copy) ----
	audioMap := map[int][2]int32{}
	for i := range streams {
		a := &streams[i]
		if a.kind != avMediaTypeAudio {
			continue
		}
		ast := fn.avformatNewStream(oc, 0)
		if ast == 0 {
			return errors.New("transcode: new audio stream")
		}
		apar := *(*uintptr)(unsafe.Pointer(ast + uintptr(f.codecparOff)))
		if rc := fn.avcodecParCopy(apar, a.par); avNeg(rc) {
			return errors.New("transcode: par copy")
		}
		setAVRational(ast, 32, a.tb[0], a.tb[1])
		audioMap[a.idx] = [2]int32{a.tb[0], a.tb[1]}
	}

	// ---- write header ----
	var hd uintptr
	if rc := fn.avformatWriteHeader(oc, &hd); avNeg(rc) {
		return errors.New("transcode: write header")
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
		setI32(dstFrame, 116, avPixFmtYUV420P)
		setI32(dstFrame, 104, int32(dw))
		setI32(dstFrame, 108, int32(dh))
		if rc := fn.avFrameGetBuffer(dstFrame, 0); avNeg(rc) {
			dstFrame = 0
		}
	}
	var frameCounter int64

	pkt := fn.avPacketAlloc()
	if pkt == 0 {
		return errors.New("transcode: pkt alloc")
	}
	defer fn.avPacketFree(&pkt)
	decFrame := fn.avFrameAlloc()
	if decFrame == 0 {
		return errors.New("transcode: frame alloc")
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
				uintptr(dw), uintptr(dh), avPixFmtYUV420P, 0x200 /*SWS_LANCZOS*/, 0, 0, 0)
			if sws == 0 {
				return errors.New("transcode: sws_getContext")
			}
			lastFmt, lastW, lastH = srcFmt, srcW, srcH
			allocDst()
			if dstFrame == 0 {
				return errors.New("transcode: dst frame buf")
			}
		}
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
			pts = pts * int64(decTb[0]) * int64(encTb[1]) / (int64(encTb[0]) * int64(decTb[1]))
		}
		setI64(dstFrame, 136, pts)

		if rc := fn.avcodecSendFrame(ectx, dstFrame); avNeg(rc) {
			return errors.New("transcode: send frame")
		}
		for {
			rp := fn.avcodecReceivePacket(ectx, pkt)
			if avNeg(rp) {
				break
			}
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
		rc := fn.avReadFrame(ifmtCtx, pkt)
		if avNeg(rc) {
			break
		}
		si := int(*(*int32)(unsafe.Pointer(pkt + 36)))
		if si == vIn.idx {
			fn.avcodecSendPacket(dctx, pkt)
			fn.avPacketUnref(pkt)
			for {
				rcf := fn.avcodecReceiveFrame(dctx, decFrame)
				if avNeg(rcf) {
					break
				}
				if e := scaleAndEncode(decFrame); e != nil {
					return e
				}
			}
		} else if tb, ok := audioMap[si]; ok {
			fn.avPacketRescaleTs(pkt, int(tb[0]), int(tb[1]), int(tb[0]), int(tb[1]))
			if rcw := fn.avInterleavedWriteFrame(oc, pkt); avNeg(rcw) {
				fn.avPacketUnref(pkt)
				return errors.New("transcode: write audio pkt")
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
			return e
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
			return errors.New("transcode: write flush pkt")
		}
		fn.avPacketUnref(pkt)
	}

	if rc := fn.avWriteTrailer(oc); avNeg(rc) {
		return errors.New("transcode: write trailer")
	}

	return nil
}

// RemuxFaststart re-muxes srcPath into dstPath with the MP4 faststart flag
// (moov atom at the front). This is a packet-level copy — no decoding or
// encoding — so it's fast and lossless. The output is a valid streaming MP4
// that browsers can play without downloading the entire file.
func (f *ffmpeg) RemuxFaststart(srcPath, dstPath string) error {
	fn := f.fn

	// ---- open input ----
	var ifmtCtx uintptr
	if rc := fn.avformatOpenInput(&ifmtCtx, cstr(srcPath), 0, nil); avNeg(rc) {
		return errors.New("faststart: open " + srcPath)
	}
	defer fn.avformatCloseInput(&ifmtCtx)

	if rc := fn.avformatFindStreamInfo(ifmtCtx, nil); avNeg(rc) {
		return errors.New("faststart: find stream info")
	}

	// ---- allocate output context (mp4) ----
	var oc uintptr
	if rc := fn.avformatAllocOutputCtx2(&oc, 0, cstr("mp4"), 0); avNeg(rc) {
		return errors.New("faststart: alloc output ctx")
	}
	defer fn.avformatFreeContext(oc)

	// ---- copy all streams ----
	nb := int(*(*uint32)(unsafe.Pointer(ifmtCtx + 44)))
	streamsPtr := *(*uintptr)(unsafe.Pointer(ifmtCtx + 48))

	type streamInfo struct {
		idx      int
		tbNum    int32
		tbDen    int32
		newIdx   int
		newTbNum int32
		newTbDen int32
	}
	streams := make([]streamInfo, 0, nb)
	for i := 0; i < nb; i++ {
		st := *(*uintptr)(unsafe.Pointer(streamsPtr + uintptr(i)*8))
		par := *(*uintptr)(unsafe.Pointer(st + uintptr(f.codecparOff)))
		num, den := getAVRational(st, 32)

		newSt := fn.avformatNewStream(oc, 0)
		if newSt == 0 {
			return errors.New("faststart: new stream")
		}
		newPar := *(*uintptr)(unsafe.Pointer(newSt + uintptr(f.codecparOff)))
		if rc := fn.avcodecParCopy(newPar, par); avNeg(rc) {
			return errors.New("faststart: par copy")
		}
		newNum, newDen := num, den
		if newNum == 0 || newDen == 0 {
			newNum, newDen = 1, 90000
		}
		setAVRational(newSt, 32, newNum, newDen)

		streams = append(streams, streamInfo{
			idx:      i,
			tbNum:    num,
			tbDen:    den,
			newIdx:   len(streams),
			newTbNum: newNum,
			newTbDen: newDen,
		})
	}

	// ---- open output file ----
	var pb uintptr
	if rc := fn.avioOpen(&pb, cstr(dstPath), 0x0002); avNeg(rc) {
		return errors.New("faststart: avio_open")
	}
	defer fn.avioClosep(&pb)
	*(*uintptr)(unsafe.Pointer(oc + 32)) = pb

	// ---- set movflags=faststart ----
	var hd uintptr
	if rc := fn.avDictSet(&hd, cstr("movflags"), cstr("faststart"), 0); avNeg(rc) {
		return errors.New("faststart: set movflags")
	}

	// ---- write header ----
	if rc := fn.avformatWriteHeader(oc, &hd); avNeg(rc) {
		if hd != 0 {
			fn.avDictFree(&hd)
		}
		return errors.New("faststart: write header")
	}
	if hd != 0 {
		fn.avDictFree(&hd)
	}

	// ---- copy packets ----
	pkt := fn.avPacketAlloc()
	if pkt == 0 {
		return errors.New("faststart: pkt alloc")
	}
	defer fn.avPacketFree(&pkt)

	for {
		rc := fn.avReadFrame(ifmtCtx, pkt)
		if avNeg(rc) {
			break
		}
		si := int(*(*int32)(unsafe.Pointer(pkt + 36)))
		if si >= 0 && si < len(streams) {
			s := streams[si]
			// Rescale timestamps from input timebase to output timebase.
			if s.tbNum != s.newTbNum || s.tbDen != s.newTbDen {
				fn.avPacketRescaleTs(pkt, int(s.tbNum), int(s.tbDen), int(s.newTbNum), int(s.newTbDen))
			}
			*(*int32)(unsafe.Pointer(pkt + 36)) = int32(s.newIdx)
			if rcw := fn.avInterleavedWriteFrame(oc, pkt); avNeg(rcw) {
				fn.avPacketUnref(pkt)
				continue
			}
		}
		fn.avPacketUnref(pkt)
	}

	// ---- write trailer ----
	if rc := fn.avWriteTrailer(oc); avNeg(rc) {
		return errors.New("faststart: write trailer")
	}

	return nil
}
