package rtp

import "testing"

func TestApplyPCMGain(t *testing.T) {
	tests := []struct {
		name   string
		input  int16
		want   int16
	}{
		{name: "triple positive sample", input: 1000, want: 3000},
		{name: "triple negative sample", input: -1000, want: -3000},
		{name: "clip positive overflow", input: 20000, want: pcmMaxSample},
		{name: "clip negative overflow", input: -20000, want: pcmMinSample},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyPCMGain(tt.input); got != tt.want {
				t.Fatalf("applyPCMGain(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeToPCM_AppliesGain(t *testing.T) {
	pcm := DecodeToPCM([]byte{0xFF}, 0)
	if len(pcm) != 1 {
		t.Fatalf("DecodeToPCM len = %d, want 1", len(pcm))
	}

	want := applyPCMGain(UlawToPCM(0xFF))
	if pcm[0] != want {
		t.Fatalf("DecodeToPCM sample = %d, want %d", pcm[0], want)
	}
}