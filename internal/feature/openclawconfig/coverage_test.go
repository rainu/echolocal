//go:build !board_biscuit

package openclawconfig

import (
	"encoding/json"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestOwnerCoverageManifest(t *testing.T) {
	data, err := os.ReadFile("../../../docs/openclaw-owner-coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Stores     map[string]map[string]struct{ Device, Web, API string }
		Actions    []string
		Exceptions []struct{ ID, Reason string }
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for name, typ := range map[string]reflect.Type{"Slot": reflect.TypeFor[clawtalk.Slot](), "EndSettings": reflect.TypeFor[talk.EndSettings](), "ConnectionSettings": reflect.TypeFor[openclaw.ConnectionSettings]()} {
		expected := map[string]bool{}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			key := strings.Split(field.Tag.Get("json"), ",")[0]
			expected[key] = true
			row, ok := manifest.Stores[name][key]
			if !ok || row.Device == "" || row.Web == "" || row.API == "" {
				t.Errorf("unreviewed owner setting %s.%s; update device/web/API coverage", name, key)
			}
		}
		for key := range manifest.Stores[name] {
			if !expected[key] {
				t.Errorf("stale coverage field %s.%s", name, key)
			}
		}
	}
	for _, action := range []string{"connect", "disconnect", "pair_again", "forget", "clear_events", "stop_talk"} {
		found := false
		for _, v := range manifest.Actions {
			found = found || v == action
		}
		if !found {
			t.Error("missing action coverage", action)
		}
	}
	if len(manifest.Exceptions) != 1 || manifest.Exceptions[0].ID != "experimental-avatar-lab" || manifest.Exceptions[0].Reason == "" {
		t.Fatal("exception must remain explicit and narrow")
	}
}
