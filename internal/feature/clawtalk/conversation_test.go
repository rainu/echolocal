//go:build !board_biscuit

package clawtalk

import (
	"context"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
	"time"
)

func TestMicIndicatorRequiresRecentFramesAndLiveOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &deviceAudio{ctx: ctx}
	now := time.Now().UnixMilli()
	if microphoneLive(true, a, now) {
		t.Fatal("connected transport falsely shows live microphone")
	}
	a.lastCaptureMS.Store(now)
	if !microphoneLive(true, a, now) {
		t.Fatal("fresh capture not shown")
	}
	if microphoneLive(false, a, now) || microphoneLive(true, a, now+501) {
		t.Fatal("inactive/stalled microphone shown live")
	}
	cancel()
	if microphoneLive(true, a, now) {
		t.Fatal("cancelled capture shown live")
	}
}
func TestCaptionRowsTranslateOrderAndRetiredOwnership(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		rows := captionRows([]talk.Caption{{Role: "user", Text: "synthetic question"}, {Role: "assistant", Text: "synthetic answer"}})
		if len(rows) != 2 || rows[0].Hint != "synthetic answer" || rows[0].Label == "clawtalk.caption_assistant" {
			t.Fatal("caption display order/translation failed")
		}
	}
	m := &Manager{active: true, generation: 2}
	m.captions.Begin(2)
	m.captionOwned(1, "user", "retired", "a")
	m.captionOwned(2, "user", "current", "b")
	if e := m.captions.Snapshot()[0].Entries; len(e) != 1 || e[0].Text != "current" {
		t.Fatal("retired captions entered current conversation")
	}
}

func TestTalkActionsUseCurrentlySelectedLanguage(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	m := &Manager{}
	for _, lang := range say.Languages() {
		say.Use(lang)
		actions := m.conversationPage().Actions
		if len(actions) != 2 || actions[0].Label != say.T("clawtalk.history") || actions[1].Label != say.T("clawtalk.end") {
			t.Fatalf("stale action locale: %s", lang)
		}
	}
}

func TestBusyTonePreviewIsInert(t *testing.T) {
	m := &Manager{active: true}
	m.PreviewTone("ding", false)
	if !m.active {
		t.Fatal("preview interrupted a conversation")
	}
}

func TestMutedConversationLabelDoesNotClaimItEnded(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	m := &Manager{active: true, forcedMuted: true, Controller: talk.New()}
	for _, lang := range say.Languages() {
		say.Use(lang)
		rows, _ := m.conversationPage().Build()
		if rows[0].Label != say.T("camera.muted") || rows[1].Label != say.T("clawtalk.mic_off") {
			t.Fatal("incorrect muted UI", lang, rows[:2])
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &deviceAudio{ctx: ctx}
	a.lastCaptureMS.Store(time.Now().UnixMilli())
	a.inputMuted.Store(true)
	if microphoneLive(true, a, time.Now().UnixMilli()) {
		t.Fatal("muted microphone shown live")
	}
}
