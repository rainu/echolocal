package config

import "testing"

func TestAvatarIntensitySnapshotsDoNotAlias(t *testing.T) {
	n := 25
	original := Config{Avatar: Avatar{MouthIntensity: &n}}
	snapshot := cloneConfig(original)
	*snapshot.Avatar.MouthIntensity = 0
	if original.Avatar.MouthPercent() != 25 {
		t.Fatal("snapshot mutated source")
	}
	if cloneConfig(Config{}).Avatar.MouthIntensity != nil {
		t.Fatal("legacy sentinel lost")
	}
}
