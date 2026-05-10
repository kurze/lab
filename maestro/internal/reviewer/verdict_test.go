package reviewer

import "testing"

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Verdict
		wantErr bool
	}{
		{
			name:  "clean PASS line",
			input: "Some review text.\n\nVERDICT: PASS\n",
			want:  Pass,
		},
		{
			name:  "clean NEEDS_FIX line",
			input: "Findings here.\n\nVERDICT: NEEDS_FIX\n",
			want:  NeedsFix,
		},
		{
			name:  "clean BLOCKER line",
			input: "Critical issues.\n\nVERDICT: BLOCKER\n",
			want:  Blocker,
		},
		{
			name:  "verdict embedded in markdown report",
			input: "# Review Report\n\n## Findings\n\n- Issue 1\n- Issue 2\n\n## Conclusion\n\nOverall the code is fine.\n\nVERDICT: PASS\n",
			want:  Pass,
		},
		{
			name:  "verdict with leading whitespace",
			input: "text\n   VERDICT: NEEDS_FIX\nmore text",
			want:  NeedsFix,
		},
		{
			name:  "verdict in bold markdown",
			input: "text\n**VERDICT: BLOCKER**\n",
			want:  Blocker,
		},
		{
			name:  "verdict under heading markers",
			input: "## VERDICT: PASS\n",
			want:  Pass,
		},
		{
			name:  "lowercase verdict value",
			input: "VERDICT: pass\n",
			want:  Pass,
		},
		{
			name:  "mixed case verdict value",
			input: "VERDICT: Needs_Fix\n",
			want:  NeedsFix,
		},
		{
			name:    "missing verdict line",
			input:   "# Review\n\nEverything looks good.\n",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unrecognised verdict value",
			input:   "VERDICT: MAYBE\n",
			wantErr: true,
		},
		{
			name:  "verdict is first line",
			input: "VERDICT: PASS\nSome trailing text.",
			want:  Pass,
		},
		{
			name:  "multiple verdict lines returns first",
			input: "VERDICT: NEEDS_FIX\nVERDICT: PASS\n",
			want:  NeedsFix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVerdict(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got verdict %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerdictString(t *testing.T) {
	tests := []struct {
		v    Verdict
		want string
	}{
		{Pass, "PASS"},
		{NeedsFix, "NEEDS_FIX"},
		{Blocker, "BLOCKER"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Verdict(%d).String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}
