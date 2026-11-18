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
