package svn

import "testing"

func TestParseUpdatedRevision(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "updated to a new revision",
			out:  "Updating '.':\nU    foo.txt\nUpdated to revision 42.\n",
			want: "42",
		},
		{
			name: "already current",
			out:  "Updating '.':\nAt revision 7.\n",
			want: "7",
		},
		{
			name: "no revision line",
			out:  "Updating '.':\n",
			want: "",
		},
		{
			name: "empty",
			out:  "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseUpdatedRevision(tc.out); got != tc.want {
				t.Errorf("parseUpdatedRevision() = %q, want %q", got, tc.want)
			}
		})
	}
}
