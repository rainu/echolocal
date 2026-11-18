package talk

import "testing"

func TestEndPhraseDoesNotMatchOrdinaryRequests(t *testing.T) {
	for _, s := range []string{"Gespräch beenden.", "END CONVERSATION!", "Termina conversazione", "Kết thúc cuộc trò chuyện"} {
		if !EndPhrase(s) {
			t.Fatal("end phrase missed")
		}
	}
	for _, s := range []string{"What does end conversation mean?", "Do not end conversation", "Bitte erkläre Gespräch beenden", ""} {
		if EndPhrase(s) {
			t.Fatal("ordinary utterance ended call")
		}
	}
}

func TestEndPhraseAcceptsSeparateCourtesyClause(t *testing.T) {
	for _, text := range []string{"Gespräch beenden, bitte!", "Gut. Unterhaltung beenden. Vielen Dank!", "Please end conversation, thank you", "Termina conversazione, grazie"} {
		if !EndPhrase(text) {
			t.Errorf("explicit command missed: %q", text)
		}
	}
	for _, text := range []string{"Gespräch nicht beenden, bitte", "Was heißt Gespräch beenden, bitte?", "Gespräch beenden, aber noch nicht", "Danke, danke"} {
		if EndPhrase(text) {
			t.Errorf("non-command accepted: %q", text)
		}
	}
}
