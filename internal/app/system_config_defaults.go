package app

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// systemConfigDefaultsJSON is the official v3.1.0 configuration-defaults
// document (GET /api/system-config/defaults), captured verbatim from a live
// immich/immich-server:v3.1.0. The endpoint describes the configuration SCHEMA
// defaults the admin settings UI renders; it carries no per-instance state, so
// mirroring the official document is the contract (the previous flat 10-key
// shape was a v1.x leftover the current web could not render).
const systemConfigDefaultsJSON = `{"backup":{"database":{"enabled":true,"cronExpression":"0 02 * * *","keepLastAmount":14}},"ffmpeg":{"crf":23,"threads":0,"preset":"ultrafast","targetVideoCodec":"h264","acceptedVideoCodecs":["h264"],"targetAudioCodec":"aac","acceptedAudioCodecs":["aac","mp3","opus"],"acceptedContainers":["mov","ogg","webm"],"targetResolution":"720","maxBitrate":"0","bframes":-1,"refs":0,"gopSize":0,"temporalAQ":false,"cqMode":"auto","twoPass":false,"preferredHwDevice":"auto","transcode":"required","tonemap":"hable","accel":"disabled","accelDecode":true,"realtime":{"enabled":false,"videoCodecs":["h264","hevc"],"resolutions":[480,720,1080]}},"integrityChecks":{"missingFiles":{"enabled":true,"cronExpression":"0 03 * * *"},"untrackedFiles":{"enabled":true,"cronExpression":"0 03 * * *"},"checksumFiles":{"enabled":true,"cronExpression":"0 03 * * *","timeLimit":3600000,"percentageLimit":1}},"job":{"backgroundTask":{"concurrency":5},"smartSearch":{"concurrency":2},"metadataExtraction":{"concurrency":5},"faceDetection":{"concurrency":2},"search":{"concurrency":5},"sidecar":{"concurrency":5},"library":{"concurrency":5},"migration":{"concurrency":5},"thumbnailGeneration":{"concurrency":3},"videoConversion":{"concurrency":1},"notifications":{"concurrency":5},"ocr":{"concurrency":1},"workflow":{"concurrency":5},"integrityCheck":{"concurrency":1},"editor":{"concurrency":2}},"logging":{"enabled":true,"level":"log"},"machineLearning":{"enabled":false,"urls":["http://immich-machine-learning:3003"],"availabilityChecks":{"enabled":true,"timeout":2000,"interval":30000},"clip":{"enabled":true,"modelName":"ViT-B-32__openai"},"duplicateDetection":{"enabled":true,"maxDistance":0.01},"facialRecognition":{"enabled":true,"modelName":"buffalo_l","minScore":0.7,"maxDistance":0.5,"minFaces":3},"ocr":{"enabled":true,"modelName":"PP-OCRv5_mobile","minDetectionScore":0.5,"minRecognitionScore":0.8,"maxResolution":736}},"map":{"enabled":true,"lightStyle":"https://tiles.immich.cloud/v1/style/light.json","darkStyle":"https://tiles.immich.cloud/v1/style/dark.json"},"reverseGeocoding":{"enabled":true},"metadata":{"faces":{"import":false}},"oauth":{"autoLaunch":false,"autoRegister":true,"buttonText":"Login with OAuth","clientId":"","clientSecret":"","defaultStorageQuota":null,"enabled":false,"issuerUrl":"","endSessionEndpoint":"","mobileOverrideEnabled":false,"mobileRedirectUri":"","prompt":"","scope":"openid email profile","signingAlgorithm":"RS256","profileSigningAlgorithm":"none","storageLabelClaim":"preferred_username","storageQuotaClaim":"immich_quota","roleClaim":"immich_role","tokenEndpointAuthMethod":"client_secret_post","timeout":30000,"allowInsecureRequests":false},"passwordLogin":{"enabled":true},"storageTemplate":{"enabled":false,"hashVerificationEnabled":true,"template":"{{y}}/{{y}}-{{MM}}-{{dd}}/{{filename}}"},"image":{"thumbnail":{"format":"webp","size":250,"quality":80,"progressive":false},"preview":{"format":"jpeg","size":1440,"quality":80,"progressive":false},"colorspace":"p3","extractEmbedded":false,"fullsize":{"enabled":false,"format":"jpeg","quality":80,"progressive":false}},"newVersionCheck":{"enabled":true,"channel":"stable"},"nightlyTasks":{"startTime":"00:00","databaseCleanup":true,"generateMemories":true,"syncQuotaUsage":true,"missingThumbnails":true,"clusterNewFaces":true},"trash":{"enabled":true,"days":30},"theme":{"customCss":""},"library":{"scan":{"enabled":true,"cronExpression":"0 0 * * *"},"watch":{"enabled":false}},"server":{"externalDomain":"","loginPageMessage":"","publicUsers":true},"notifications":{"smtp":{"enabled":false,"from":"","replyTo":"","transport":{"ignoreCert":false,"host":"","port":587,"secure":false,"username":"","password":""}}},"templates":{"email":{"welcomeTemplate":"","albumInviteTemplate":"","albumUpdateTemplate":""}},"user":{"deleteDelay":7}}`

// systemConfigDefaults is the parsed document; parsing at init makes a
// malformed constant fail fast instead of at request time.
var systemConfigDefaults = func() map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(systemConfigDefaultsJSON), &m); err != nil {
		panic("system-config defaults: " + err.Error())
	}
	return m
}()

// handleSystemConfigDefaults mirrors GET /api/system-config/defaults.
func (a *App) handleSystemConfigDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, systemConfigDefaults)
}
