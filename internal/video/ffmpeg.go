//go:build linux || darwin

package video

// ffmpeg.go — software video backend implemented by loading the FFmpeg
// shared libraries (libavformat/libavcodec/libavutil/libswscale/libswresample)
// in-process via github.com/ebitengine/purego. NO CGO, NO CLI.
//
// The implementation targets the FFmpeg C API across the 4.4–7.x line. It
// avoids replicating most struct layouts by relying only on stable leading
// fields (AVFrame.data[0]/linesize[0], AVCodecParameters.width/height,
// AVFormatContext.pb) and opaque pointers for everything else.
//
// If any required library fails to load, newFFmpeg returns
// errBackendUnavailable and the selector falls back to the placeholder.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ---- FFmpeg constants ----
const (
	avPixFmtYUV420P = 0
	avPixFmtRGBA    = 26 // AV_PIX_FMT_RGBA (same enum value on 7.1)
	avMediaTypeVideo = 0
	avMediaTypeAudio = 1
	avOptFlag       = 0
	avIoFlagWrite   = 1
	avfmtFlagCustomIO = 0x8000 // AVFMT_FLAG_CUSTOM_IO
	seekSet = 0
	seekCur = 1
	seekEnd = 2
	avCodecIDH264 = 27
	avCodecIDAAC  = 86018
	avNoPtsValue  = -0x8000000000000000 // AV_NOPTS_VALUE
	avTimebaseDen = 30000

	// FFmpeg 7.1 ABI: AVCodecContext.profile sits at offset 688 (verified via
	// offsetof). Set the *field*, not the "profile" dict option: libx264 routes
	// the dict through its own Eval parser, which rejects plain names such as
	// "high" (rc=-22, "Undefined constant or missing '(' in 'high'").
	avCodecCtxProfile = 688
	ffProfileH264Main = 77
	ffProfileH264High = 100
)

// avNeg reports whether an FFmpeg int return code is an error (<0). FFmpeg
// returns a 32-bit int; purego surfaces it as a zero-extended Go int, so a
// negative AVERROR (e.g. -11 EAGAIN -> 4294967285) looks positive. Reinterpret
// the low 32 bits as signed before comparing.
func avNeg(rc int) bool { return int32(rc) < 0 }

// ---- C function pointers (registered at load time) ----
type ffFuncs struct {
	// avutil
	avMalloc             func(size uintptr) uintptr
	avFree               func(ptr uintptr)
	avFrameAlloc         func() uintptr
	avFrameFree          func(f *uintptr)
	avFrameGetBuffer     func(frame uintptr, align int) int
	avFrameUnref         func(frame uintptr)
	avPacketAlloc        func() uintptr
	avPacketFree         func(p *uintptr)
	avcodecParAlloc      func() uintptr
	avcodecParFree       func(p *uintptr)
	avcodecParToContext  func(ctx, par uintptr) int
	avcodecParCopy       func(dst, src uintptr) int
	avDictSet            func(d *uintptr, key, val uintptr, flags int) int
	avStrerror           func(err int, buf uintptr, sz uintptr) int
	avLogSetLevel        func(level int)
	// avcodec
	avcodecFindDecoder   func(id int) uintptr
	avcodecFindEncoder   func(id int) uintptr
	avcodecFindEncoderByName func(name uintptr) uintptr
	avDictFree           func(d *uintptr)
	avcodecAllocContext3 func(codec uintptr) uintptr
	avcodecOpen2         func(ctx, codec uintptr, d *uintptr) int
	avcodecSendPacket    func(ctx, pkt uintptr) int
	avcodecReceiveFrame  func(ctx, frame uintptr) int
	avcodecSendFrame     func(ctx, frame uintptr) int
	avcodecReceivePacket func(ctx, pkt uintptr) int
	// avformat
	avformatAllocContext  func() uintptr
	avformatOpenInput     func(ps *uintptr, url uintptr, fmt uintptr, d *uintptr) int
	avformatFindStreamInfo func(ctx uintptr, d *uintptr) int
	avReadFrame           func(ctx, pkt uintptr) int
	avformatCloseInput    func(ps *uintptr)
	avformatNewStream     func(ctx, codec uintptr) uintptr
	avformatAllocOutputCtx2 func(ps *uintptr, ofmt uintptr, name, url uintptr) int
	avioOpen              func(ps *uintptr, url uintptr, flags int) int
	avioClosep           func(ps *uintptr)
	avWriteFrame          func(ctx, pkt uintptr) int
	avInterleavedWriteFrame func(ctx, pkt uintptr) int
	avWriteTrailer        func(ctx uintptr) int
	avformatWriteHeader   func(ctx uintptr, d *uintptr) int
	avformatFreeContext   func(ctx uintptr)
	avcodecFreeContext    func(cctx *uintptr)
	avOptSet              func(obj, name, val uintptr, flags int) int
	avPacketRescaleTs     func(pkt uintptr, srcNum, srcDen, dstNum, dstDen int)
	// swscale
	swsGetContext func(sw, sh, sf, dw, dh, df, flags, sf2, df2, p uintptr) uintptr
	swsScale     func(ctx, srcSlice, srcStride uintptr, sy, sh int, dst, dstStride uintptr) int
	swsFreeContext func(ctx uintptr)
	// misc
	avformatVersion func() int
	avcodecVersion  func() int
	avcodecParFromCtx func(par, ctx uintptr) int
	avRescaleQ       func(a int64, bq, cq uintptr) int64
	avPacketUnref    func(pkt uintptr)
}

type ffmpeg struct {
	libs        []uintptr // keep handles alive
	fn          *ffFuncs
	codecparOff int // offset of AVStream.codecpar (ABI differs across ffmpeg)
	// encoder selection (resolved once at load; HW-first with software fallback)
	selectedName string // e.g. "h264_nvenc" or "libx264"
	nameLabel    string // e.g. "nvenc" or "software"
	selectedCodec uintptr
	profileName  string // "high" or "main"
}

func (f *ffmpeg) Name() string        { return "ffmpeg-" + f.nameLabel }
func (f *ffmpeg) HardwareAccel() bool { return f.nameLabel != "software" }

// ---- library loading ----

func loadFFmpeg() (*ffmpeg, error) {
	cands := map[string][]string{
		"libavformat":   {"libavformat.so", "libavformat.so.61", "libavformat.so.60", "libavformat.so.58", "libavformat.dylib", "avformat-61.dll", "avformat-60.dll", "avformat-58.dll"},
		"libavcodec":    {"libavcodec.so", "libavcodec.so.61", "libavcodec.so.60", "libavcodec.so.58", "libavcodec.dylib", "avcodec-61.dll", "avcodec-60.dll", "avcodec-58.dll"},
		"libavutil":     {"libavutil.so", "libavutil.so.59", "libavutil.so.58", "libavutil.so.56", "libavutil.dylib", "avutil-59.dll", "avutil-58.dll", "avutil-56.dll"},
		"libswscale":    {"libswscale.so", "libswscale.so.8", "libswscale.so.7", "libswscale.so.6", "libswscale.so.5", "libswscale.dylib", "swscale-8.dll", "swscale-7.dll", "swscale-6.dll", "swscale-5.dll"},
		"libswresample": {"libswresample.so", "libswresample.so.5", "libswresample.so.4", "libswresample.so.3", "libswresample.dylib", "swresample-5.dll", "swresample-4.dll", "swresample-3.dll"},
	}

	exeDir, _ := os.Executable()
	searchDirs := []string{".", filepath.Dir(exeDir), filepath.Join(filepath.Dir(exeDir), "libs")}

	f := &ffmpeg{fn: &ffFuncs{}}
	loaded := map[string]uintptr{}

	open := func(name string) (uintptr, error) {
		for _, d := range searchDirs {
			p := filepath.Join(d, name)
			if h, err := purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				return h, nil
			}
		}
		// also try system-default lookup by soname
		return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	}

	for lib, names := range cands {
		var h uintptr
		var err error
		for _, n := range names {
			h, err = open(n)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, errBackendUnavailable
		}
		loaded[lib] = h
		f.libs = append(f.libs, h)
	}

	fn := f.fn
	// libavutil
	purego.RegisterLibFunc(&fn.avMalloc, loaded["libavutil"], "av_malloc")
	purego.RegisterLibFunc(&fn.avFree, loaded["libavutil"], "av_free")
	purego.RegisterLibFunc(&fn.avFrameAlloc, loaded["libavutil"], "av_frame_alloc")
	purego.RegisterLibFunc(&fn.avFrameFree, loaded["libavutil"], "av_frame_free")
	purego.RegisterLibFunc(&fn.avFrameGetBuffer, loaded["libavutil"], "av_frame_get_buffer")
	purego.RegisterLibFunc(&fn.avFrameUnref, loaded["libavutil"], "av_frame_unref")
	purego.RegisterLibFunc(&fn.avDictSet, loaded["libavutil"], "av_dict_set")
	purego.RegisterLibFunc(&fn.avStrerror, loaded["libavutil"], "av_strerror")
	purego.RegisterLibFunc(&fn.avLogSetLevel, loaded["libavutil"], "av_log_set_level")
	purego.RegisterLibFunc(&fn.avRescaleQ, loaded["libavutil"], "av_rescale_q")
	// libavcodec
	purego.RegisterLibFunc(&fn.avPacketAlloc, loaded["libavcodec"], "av_packet_alloc")
	purego.RegisterLibFunc(&fn.avPacketFree, loaded["libavcodec"], "av_packet_free")
	purego.RegisterLibFunc(&fn.avPacketUnref, loaded["libavcodec"], "av_packet_unref")
	purego.RegisterLibFunc(&fn.avcodecParAlloc, loaded["libavcodec"], "avcodec_parameters_alloc")
	purego.RegisterLibFunc(&fn.avcodecParFree, loaded["libavcodec"], "avcodec_parameters_free")
	purego.RegisterLibFunc(&fn.avcodecParToContext, loaded["libavcodec"], "avcodec_parameters_to_context")
	purego.RegisterLibFunc(&fn.avcodecParFromCtx, loaded["libavcodec"], "avcodec_parameters_from_context")
	purego.RegisterLibFunc(&fn.avcodecParCopy, loaded["libavcodec"], "avcodec_parameters_copy")
	purego.RegisterLibFunc(&fn.avPacketRescaleTs, loaded["libavcodec"], "av_packet_rescale_ts")
	purego.RegisterLibFunc(&fn.avcodecFindDecoder, loaded["libavcodec"], "avcodec_find_decoder")
	purego.RegisterLibFunc(&fn.avcodecFindEncoder, loaded["libavcodec"], "avcodec_find_encoder")
	purego.RegisterLibFunc(&fn.avcodecFindEncoderByName, loaded["libavcodec"], "avcodec_find_encoder_by_name")
	purego.RegisterLibFunc(&fn.avDictFree, loaded["libavcodec"], "av_dict_free")
	purego.RegisterLibFunc(&fn.avcodecAllocContext3, loaded["libavcodec"], "avcodec_alloc_context3")
	purego.RegisterLibFunc(&fn.avcodecOpen2, loaded["libavcodec"], "avcodec_open2")
	purego.RegisterLibFunc(&fn.avcodecSendPacket, loaded["libavcodec"], "avcodec_send_packet")
	purego.RegisterLibFunc(&fn.avcodecReceiveFrame, loaded["libavcodec"], "avcodec_receive_frame")
	purego.RegisterLibFunc(&fn.avcodecSendFrame, loaded["libavcodec"], "avcodec_send_frame")
	purego.RegisterLibFunc(&fn.avcodecReceivePacket, loaded["libavcodec"], "avcodec_receive_packet")
	// libavformat
	purego.RegisterLibFunc(&fn.avformatAllocContext, loaded["libavformat"], "avformat_alloc_context")
	purego.RegisterLibFunc(&fn.avformatOpenInput, loaded["libavformat"], "avformat_open_input")
	purego.RegisterLibFunc(&fn.avformatFindStreamInfo, loaded["libavformat"], "avformat_find_stream_info")
	purego.RegisterLibFunc(&fn.avReadFrame, loaded["libavformat"], "av_read_frame")
	purego.RegisterLibFunc(&fn.avformatCloseInput, loaded["libavformat"], "avformat_close_input")
	purego.RegisterLibFunc(&fn.avformatNewStream, loaded["libavformat"], "avformat_new_stream")
	purego.RegisterLibFunc(&fn.avformatAllocOutputCtx2, loaded["libavformat"], "avformat_alloc_output_context2")
	purego.RegisterLibFunc(&fn.avioOpen, loaded["libavformat"], "avio_open")
	purego.RegisterLibFunc(&fn.avioClosep, loaded["libavformat"], "avio_closep")
	purego.RegisterLibFunc(&fn.avWriteFrame, loaded["libavformat"], "av_write_frame")
	purego.RegisterLibFunc(&fn.avInterleavedWriteFrame, loaded["libavformat"], "av_interleaved_write_frame")
	purego.RegisterLibFunc(&fn.avWriteTrailer, loaded["libavformat"], "av_write_trailer")
	purego.RegisterLibFunc(&fn.avformatWriteHeader, loaded["libavformat"], "avformat_write_header")
	purego.RegisterLibFunc(&fn.avformatFreeContext, loaded["libavformat"], "avformat_free_context")
	purego.RegisterLibFunc(&fn.avformatVersion, loaded["libavformat"], "avformat_version")
	// libavcodec
	purego.RegisterLibFunc(&fn.avcodecFreeContext, loaded["libavcodec"], "avcodec_free_context")
	purego.RegisterLibFunc(&fn.avOptSet, loaded["libavcodec"], "av_opt_set")
	// libswscale
	purego.RegisterLibFunc(&fn.swsGetContext, loaded["libswscale"], "sws_getContext")
	purego.RegisterLibFunc(&fn.swsScale, loaded["libswscale"], "sws_scale")
	purego.RegisterLibFunc(&fn.swsFreeContext, loaded["libswscale"], "sws_freeContext")
	// libavcodec (version)
	purego.RegisterLibFunc(&fn.avcodecVersion, loaded["libavcodec"], "avcodec_version")

	// AVStream.codecpar offset. Verified against the pinned FFmpeg 7.1
	// headers (aarch64): AVStream = { AVClass* av_class(0), int index(8),
	// int id(12), AVCodecParameters* codecpar(16), ... }. The deprecated
	// AVStream.codec field was removed in 5.0, so for 5.0+ codecpar sits at
	// offset 16. (Older 4.4 kept codec before codecpar, pushing it much
	// further; we target 5.0+.)
	f.codecparOff = 16
	return f, nil
}

func newFFmpeg() (Processor, error) {
	f, err := loadFFmpeg()
	if err != nil {
		return nil, err
	}
	// quiet logs
	f.fn.avLogSetLevel(0) // AV_LOG_QUIET
	f.selectEncoder()
	return f, nil
}

// totalMemGB returns total system RAM in gibibytes (best-effort; Linux
// /proc/meminfo, otherwise 0 which forces the conservative "main" profile).
func totalMemGB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				if kb, e := strconv.ParseInt(f[1], 10, 64); e == nil {
					return int(kb / (1024 * 1024))
				}
			}
		}
	}
	return 0
}

// capableMachine reports whether this host should use the higher-quality
// "high" H.264 profile: >=4 CPU cores AND >=8 GiB RAM. Otherwise "main".
func (f *ffmpeg) capableMachine() bool {
	return runtime.NumCPU() >= 4 && totalMemGB() >= 8
}

// profileValue maps the resolved profile name to the H.264 profile
// enum (FF_PROFILE_H264_MAIN/HIGH as int). The encoder context's `profile`
// field must be set directly; the dict option breaks libx264 (see comment on
// avCodecCtxProfile).
func (f *ffmpeg) profileValue() int32 {
	if f.profileName == "high" {
		return ffProfileH264High
	}
	return ffProfileH264Main
}

// selectEncoder probes hardware H.264 encoders in priority order
// (VideoToolbox -> NVENC -> QSV -> AMF) and selects the first that opens;
// otherwise falls back to the pure-software libx264 encoder. The chosen
// profile ("high" on capable machines, else "main") is stored for the
// actual transcode.
func (f *ffmpeg) selectEncoder() {
	f.profileName = "main"
	if f.capableMachine() {
		f.profileName = "high"
	}

	// priority order required by the project goal
	cands := []struct{ name, label string }{
		{"h264_videotoolbox", "videotoolbox"},
		{"h264_nvenc", "nvenc"},
		{"h264_qsv", "qsv"},
		{"h264_amf", "amf"},
	}
	for _, c := range cands {
		if f.probeEncoderByName(c.name) {
			f.selectedName = c.name
			f.nameLabel = c.label
			f.selectedCodec = f.fn.avcodecFindEncoderByName(cstr(c.name))
			return
		}
	}
	// software fallback (always available)
	f.selectedName = "libx264"
	f.nameLabel = "software"
	f.selectedCodec = f.fn.avcodecFindEncoder(avCodecIDH264)
}

// probeEncoderByName checks whether an encoder can actually be opened (not
// just that the symbol exists) by allocating a throwaway context, setting the
// resolved profile, and attempting avcodec_open2 with a tiny frame. A real
// device/driver is required, so unavailable HW encoders fail here and we move
// on to the next candidate.
func (f *ffmpeg) probeEncoderByName(name string) bool {
	fn := f.fn
	codec := fn.avcodecFindEncoderByName(cstr(name))
	if codec == 0 {
		return false
	}
	c := fn.avcodecAllocContext3(codec)
	if c == 0 {
		return false
	}
	setI32(c, 12, avMediaTypeVideo) // codec_type
	setI64(c, 16, int64(codec))     // codec
	setI32(c, 116, 320)             // width
	setI32(c, 120, 240)             // height
	setI32(c, 140, avPixFmtYUV420P) // pix_fmt (offsetof @140 on 7.1)
	setI32(c, avCodecCtxProfile, f.profileValue())
	var opts uintptr
	rc := fn.avcodecOpen2(c, codec, &opts)
	if opts != 0 {
		fn.avDictFree(&opts)
	}
	fn.avcodecFreeContext(&c)
	return !avNeg(rc)
}

// ---- helpers for struct field access (ffmpeg 7.1, 64-bit LE) ----
// AVFrame: data[8] (64 bytes), linesize[8] (32 bytes) -> linesize[0] @64
func frameData0(frame uintptr) uintptr { return *(*uintptr)(unsafe.Pointer(frame)) }
func frameStride0(frame uintptr) int32 { return *(*int32)(unsafe.Pointer(frame + 64)) }
func frameW(frame uintptr) int32       { return *(*int32)(unsafe.Pointer(frame + 104)) }
func frameH(frame uintptr) int32       { return *(*int32)(unsafe.Pointer(frame + 108)) }

// AVCodecParameters: width @72, height @76 (FFmpeg 7.1 ABI, verified via
// offsetof against the pinned headers — see loadFFmpeg comment).
func parW(par uintptr) int32 { return *(*int32)(unsafe.Pointer(par + 72)) }
func parH(par uintptr) int32 { return *(*int32)(unsafe.Pointer(par + 76)) }

// ---- generic little-endian field setters (verified offsets, 64-bit LE) ----
func setI32(p uintptr, off int, v int32) { *(*int32)(unsafe.Pointer(p + uintptr(off))) = v }
func setI64(p uintptr, off int, v int64) { *(*int64)(unsafe.Pointer(p + uintptr(off))) = v }
func getI64(p uintptr, off int) int64     { return *(*int64)(unsafe.Pointer(p + uintptr(off))) }

// setAVRational writes an AVRational {num,den} (8 bytes) at off.
// getAVRational reads it back.
func setAVRational(p uintptr, off int, num, den int32) {
	*(*int32)(unsafe.Pointer(p + uintptr(off))) = num
	*(*int32)(unsafe.Pointer(p + uintptr(off+4))) = den
}
func getAVRational(p uintptr, off int) (num, den int32) {
	return *(*int32)(unsafe.Pointer(p + uintptr(off))), *(*int32)(unsafe.Pointer(p + uintptr(off+4)))
}

// AVCodecContext field accessors (offsets verified via offsetof on 7.1/aarch64:
// ctx->time_base @84, framerate @100, gop_size @332, pix_fmt @140,
// max_b_frames @200, thread_count @656; AVFrame.format @116, AVFrame.pts @136)
func ctxSetI32(c uintptr, off int, v int32)  { setI32(c, off, v) }
func ctxFrameFormat(frame uintptr) int32     { return *(*int32)(unsafe.Pointer(frame + 116)) }
func ctxFramePts(frame uintptr) int64        { return getI64(frame, 136) }

// AVPacket field accessors (pts@8, dts@16, duration@64 — verified)
func pktPts(pkt uintptr) int64   { return getI64(pkt, 8) }
func pktDts(pkt uintptr) int64   { return getI64(pkt, 16) }
func pktDur(pkt uintptr) int64   { return getI64(pkt, 64) }
func setPktPts(pkt uintptr, v int64) { setI64(pkt, 8, v) }
func setPktDts(pkt uintptr, v int64) { setI64(pkt, 16, v) }
func setPktDur(pkt uintptr, v int64) { setI64(pkt, 64, v) }

// cstr returns a NUL-terminated C string pointer that stays valid for the
// lifetime of the process. CRITICAL: the backing memory is kept alive in a
// package-level slice. Returning uintptr(unsafe.Pointer(&localSlice[0]))
// directly is a use-after-free hazard under purego — once the local slice
// leaves scope the GC may reclaim it before the C function reads it.
var cstrKeepAlive [][]byte

func cstr(s string) uintptr {
	b := append([]byte(s), 0)
	cstrKeepAlive = append(cstrKeepAlive, b) // keep backing array alive
	return uintptr(unsafe.Pointer(&b[0]))
}

// openInput opens an AVFormatContext over the given bytes. The source is
// materialised to a temporary file and opened through libavformat's native
// file protocol so that seeking works reliably: the MOV/MP4 demuxer reads the
// sample tables after avformat_find_stream_info has consumed the stream to
// EOF, which the previous in-memory custom-AVIO path could not satisfy
// (av_read_frame returned an error immediately after find_stream_info).
//
// This stays fully in-process: no CGO, no external CLI, no network. The temp
// file is only a local I/O sink that FFmpeg reads back; the cleanup func
// removes it. This deliberately mirrors the mux side (ffmpeg_transcode.go),
// which already uses a temp file because the in-memory AVIO output path is
// ABI-fragile across FFmpeg builds.
func (f *ffmpeg) openInput(src []byte) (ctx uintptr, cleanup func(), err error) {
	fn := f.fn

	tmp, err := os.CreateTemp("", "immich-vid-*.tmp")
	if err != nil {
		return 0, nil, errors.New("open input: temp file: " + err.Error())
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(src); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return 0, nil, errors.New("open input: write temp: " + werr.Error())
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return 0, nil, errors.New("open input: close temp: " + cerr.Error())
	}

	ctx = fn.avformatAllocContext()
	if ctx == 0 {
		os.Remove(tmpName)
		return 0, nil, errors.New("avformat_alloc_context failed")
	}
	var pctx uintptr = ctx
	if rc := fn.avformatOpenInput(&pctx, cstr(tmpName), 0, nil); avNeg(rc) {
		fn.avformatCloseInput(&pctx)
		os.Remove(tmpName)
		return 0, nil, errors.New("avformat_open_input failed")
	}
	if rc := fn.avformatFindStreamInfo(ctx, nil); avNeg(rc) {
		fn.avformatCloseInput(&pctx)
		os.Remove(tmpName)
		return 0, nil, errors.New("avformat_find_stream_info failed")
	}
	cleanup = func() {
		fn.avformatCloseInput(&pctx)
		os.Remove(tmpName)
	}
	return ctx, cleanup, nil
}
