//go:build !board_biscuit

// Package assistantconfig owns local wake selection shared by device and web.
package assistantconfig

import (
	"errors"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/detect"
	"github.com/ygelfand/echolocal/internal/feature/voice"
	"github.com/ygelfand/echolocal/internal/lib/wake"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func WakeChoices() []clawtalk.Choice {
	out := []clawtalk.Choice{{Value: "", Label: say.T("assistants.off")}}
	for _, m := range wake.Lib().Ours() {
		out = append(out, clawtalk.Choice{Value: m.ID, Label: m.Phrase})
	}
	return out
}
func WakeIDs() []string { return []string{config.Get().Wake.Slot(0).ID, config.Get().Wake.Slot(1).ID} }
func SetWakeWord(slot int, id string) error {
	if slot < 0 || slot > 1 {
		return errors.New("talk_selection_invalid")
	}
	found := false
	for _, v := range WakeChoices() {
		if v.Value == id {
			found = true
		}
	}
	if !found {
		return errors.New("talk_selection_invalid")
	}
	if err := config.Set().Wake(slot).ID(id); err != nil {
		return err
	}
	slots := voice.Get().Slots()
	for len(slots) <= slot {
		slots = append(slots, "")
	}
	slots[slot] = id
	voice.Get().SetSlots(slots)
	detect.Get().Load()
	return nil
}
