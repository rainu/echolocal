package avatar

import (
	"math"
	"time"
)

// SpeechMouth scales only the final visual expression. Never feed this value
// back into the speaker, capture, VAD or provider transport.
func SpeechMouth(level, intensity float64) float64 {
	if !finite(level) || !finite(intensity) {
		return 0
	}
	return math.Max(0, math.Min(1, level)) * math.Max(0, math.Min(1, intensity))
}

// PreviewLevel replays an offline synthesized speech amplitude envelope, without
// sound, microphone capture or a provider connection. 100 ms RMS windows use the
// same logarithmic mapping as the playback meter (noise floor .00012, span 100).
// Source: espeak-ng en, 155 wpm, PCM attenuated to 2% before RMS analysis.
// Text: "Hello. Here is a quiet sentence. This is my normal speaking voice.
// Now a more lively phrase. And a short pause."
func PreviewLevel(elapsed time.Duration) float64 {
	if elapsed < 0 {
		return 0
	}
	return speechPreview[int(elapsed/(100*time.Millisecond))%len(speechPreview)]
}

var speechPreview = [...]float64{.503, .582, .728, .653, .533, 0, 0, 0, .127, .67, .592, .642, .613, .289, .722, .67, .358, .337, .644, .579, .347, .614, .434, .292, 0, 0, 0, .18, .633, .576, .575, .648, .528, .761, .529, .602, .32, .635, .521, .6, .568, .308, .714, .71, .491, .3, 0, 0, 0, .426, .729, .699, .597, .731, .656, .597, .756, .463, .573, .545, .6, .646, .55, .346, .04, 0, 0, .132, .641, .518, .56, .626, .752, .38, .168, .698, .713, .486, .298, 0, 0, 0, 0}
