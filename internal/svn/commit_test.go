package svn

import "testing"

func TestParseCommittedRevision(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "typical commit",
			out:  "Sending        foo.txt\nTransmitting file data .\nCommitted revision 128.\n",
			want: "128",
		},
		{
			name: "add only",
			out:  "Adding         bar\nCommitted revision 7.",
			want: "7",
		},
		{
			name: "no revision line",
			out:  "Sending        foo.txt\n",
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
			if got := parseCommittedRevision(tc.out); got != tc.want {
				t.Errorf("parseCommittedRevision() = %q, want %q", got, tc.want)
			}
		})
	}
}
