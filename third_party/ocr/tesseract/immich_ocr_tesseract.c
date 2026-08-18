#include <stdlib.h>
#include <string.h>
#include <tesseract/capi.h>
#include <leptonica/allheaders.h>

static const char *normalize_language(const char *language) {
    if (!language || language[0] == '\0') return "eng";
    if (strcmp(language, "zh-Hans") == 0 || strcmp(language, "zh-CN") == 0) return "chi_sim";
    if (strcmp(language, "zh-Hant") == 0 || strcmp(language, "zh-TW") == 0) return "chi_tra";
    return language;
}

int immich_ocr_recognize(const unsigned char *data, size_t length,
                         const char *mime, const char *language,
                         char **result, size_t *resultLength) {
    (void)mime;
    if (!data || length == 0 || !result || !resultLength) return 2;
    TessBaseAPI *api = TessBaseAPICreate();
    if (!api) return 3;
    const char *lang = normalize_language(language);
    if (TessBaseAPIInit3(api, NULL, lang) != 0) {
        TessBaseAPIDelete(api);
        return 4;
    }
    struct Pix *pix = pixReadMem(data, length);
    if (!pix) {
        TessBaseAPIEnd(api);
        TessBaseAPIDelete(api);
        return 5;
    }
    TessBaseAPISetImage2(api, pix);
    char *text = TessBaseAPIGetUTF8Text(api);
    size_t n = text ? strlen(text) : 0;
    if (!text || n == 0) {
        if (text) TessDeleteText(text);
        pixDestroy(&pix);
        TessBaseAPIEnd(api);
        TessBaseAPIDelete(api);
        return 6;
    }
    *result = malloc(n);
    if (!*result) {
        TessDeleteText(text);
        pixDestroy(&pix);
        TessBaseAPIEnd(api);
        TessBaseAPIDelete(api);
        return 7;
    }
    memcpy(*result, text, n);
    *resultLength = n;
    TessDeleteText(text);
    pixDestroy(&pix);
    TessBaseAPIEnd(api);
    TessBaseAPIDelete(api);
    return 0;
}

void immich_ocr_free(void *ptr) { free(ptr); }
