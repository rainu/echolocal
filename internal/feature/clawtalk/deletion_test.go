//go:build !board_biscuit

package clawtalk

import (
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"testing"
)

func TestDeleteConfirmationAndOpenDetailDoNotRetainDeletedText(t *testing.T) {
	sh := shellfeature.Get()
	defer sh.Close()
	m := &Manager{generation: 1}
	m.captions.Begin(1)
	m.captions.Append(1, "user", "synthetic private caption", "a")
	rows, _ := m.HistoryPage().Build()
	detail := rows[0].Open().(*shell.Page)
	m.deleteHistoryAction(1).Do()
	confirm := sh.Top().(*shell.Page)
	if confirm.Actions[1].Label != m.deleteHistoryAction(1).Label {
		t.Fatal("delete confirmation used a different destructive action label")
	}
	confirm.Actions[0].Do()
	if len(m.captions.Snapshot()) != 1 {
		t.Fatal("cancel deleted history")
	}
	m.deleteHistoryAction(1).Do()
	sh.Top().(*shell.Page).Actions[1].Do()
	if len(m.captions.Snapshot()) != 0 {
		t.Fatal("confirmed delete did not erase")
	}
	after, _ := detail.Build()
	if len(after) != 1 || after[0].Label != say.T("clawtalk.caption_empty") {
		t.Fatal("open detail retained deleted caption")
	}
	// Deleting the latest display must not reveal an older conversation as current.
	m.captions.Begin(2)
	m.captions.Append(2, "user", "synthetic old text", "b")
	m.captions.Begin(3)
	m.captions.Remove(3)
	m.generation = 3
	m.Controller = talk.New()
	liveRows, _ := m.conversationPage().Build()
	if liveRows[len(liveRows)-1].Label != say.T("clawtalk.caption_empty") {
		t.Fatal("deleted current history revealed an older conversation")
	}
	m.deleteHistoryAction(0).Do()
	sh.Top().(*shell.Page).Actions[1].Do()
	if len(m.captions.Snapshot()) != 0 {
		t.Fatal("clear all did not erase")
	}
}
