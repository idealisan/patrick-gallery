package app

import "testing"

// The official v3.1.0 GET /api/system-config/defaults returns the full
// configuration-schema tree (verified live). The previous flat 10-key shape
// was a v1.x leftover the current web cannot render, so this guards the
// official group set.
func TestSystemConfigDefaultsMatchesOfficialShape(t *testing.T) {
	want := []string{
		"backup", "ffmpeg", "image", "integrityChecks", "job", "library",
		"logging", "machineLearning", "map", "metadata", "newVersionCheck",
		"nightlyTasks", "notifications", "oauth", "passwordLogin",
		"reverseGeocoding", "server", "storageTemplate", "templates", "theme",
		"trash", "user",
	}
	for _, g := range want {
		if _, ok := systemConfigDefaults[g]; !ok {
			t.Fatalf("official v3.1.0 defaults must carry group %q", g)
		}
	}
	if len(systemConfigDefaults) != len(want) {
		t.Fatalf("expected %d groups, got %d", len(want), len(systemConfigDefaults))
	}
	// A couple of nested values the admin UI reads directly.
	backup, _ := systemConfigDefaults["backup"].(map[string]any)
	db, _ := backup["database"].(map[string]any)
	if db["keepLastAmount"] == nil {
		t.Fatalf("backup.database.keepLastAmount missing: %v", backup)
	}
	ff, _ := systemConfigDefaults["ffmpeg"].(map[string]any)
	if ff["targetVideoCodec"] == nil {
		t.Fatalf("ffmpeg.targetVideoCodec missing: %v", ff)
	}
}
