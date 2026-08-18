#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#import <ImageIO/ImageIO.h>
#import <CoreGraphics/CoreGraphics.h>

// Stable C ABI consumed by internal/ocr through purego. The shim owns the
// returned buffer; callers must release it with immich_ocr_free.
int immich_ocr_recognize(const unsigned char *data, size_t length,
                         const char *language, char **result, size_t *resultLength) {
    if (!data || length == 0 || !result || !resultLength) return 2;
    @autoreleasepool {
        NSData *imageData = [NSData dataWithBytes:data length:length];
        CGImageSourceRef source = CGImageSourceCreateWithData((__bridge CFDataRef)imageData, NULL);
        if (!source) return 3;
        CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, NULL);
        CFRelease(source);
        if (!image) return 3;

        __block NSMutableArray<NSString *> *lines = [NSMutableArray array];
        VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] initWithCompletionHandler:
            ^(VNRequest *req, NSError *error) {
                if (error) return;
                for (VNRecognizedTextObservation *observation in req.results) {
                    VNRecognizedText *candidate = [observation topCandidates:1].firstObject;
                    if (candidate.string.length > 0) [lines addObject:candidate.string];
                }
            }];
        request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
        request.usesLanguageCorrection = YES;
        if (language && strlen(language) > 0) {
            NSString *lang = [NSString stringWithUTF8String:language];
            if (lang) request.recognitionLanguages = @[lang];
        }

        VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCGImage:image options:@{}];
        NSError *error = nil;
        BOOL ok = [handler performRequests:@[request] error:&error];
        CGImageRelease(image);
        if (!ok || error) return 4;

        NSString *text = [lines componentsJoinedByString:@"\n"];
        NSDictionary *payload = @{ @"text": text ?: @"" };
        NSData *json = [NSJSONSerialization dataWithJSONObject:payload options:0 error:&error];
        if (!json || error) return 5;
        char *buffer = malloc(json.length);
        if (!buffer) return 6;
        memcpy(buffer, json.bytes, json.length);
        *result = buffer;
        *resultLength = json.length;
        return 0;
    }
}

void immich_ocr_free(void *ptr) { free(ptr); }
