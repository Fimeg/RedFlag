package system

import (
	"reflect"
	"testing"
)

func TestDecodeCapabilities(t *testing.T) {
	cases := []struct {
		name string
		mask string
		want []string
	}{
		{name: "unprivileged", mask: "0000000000000000", want: nil},
		{name: "empty", mask: "", want: nil},
		{name: "whitespace is trimmed", mask: "  0000000000000001  ", want: []string{"CAP_CHOWN"}},
		{name: "single low bit", mask: "0000000000000001", want: []string{"CAP_CHOWN"}},
		{name: "net bind service", mask: "0000000000000400", want: []string{"CAP_NET_BIND_SERVICE"}},
		{name: "sys admin", mask: "0000000000200000", want: []string{"CAP_SYS_ADMIN"}},
		{
			name: "a container's default docker set",
			mask: "00000000a80425fb",
			want: []string{
				"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_FSETID",
				"CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SETPCAP",
				"CAP_NET_BIND_SERVICE", "CAP_NET_RAW", "CAP_SYS_CHROOT",
				"CAP_MKNOD", "CAP_AUDIT_WRITE", "CAP_SETFCAP",
			},
		},
		{name: "beyond the known table stays legible", mask: "0000020000000000", want: []string{"CAP_41"}},
		{name: "not hex", mask: "not-a-mask", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeCapabilities(tc.mask)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decodeCapabilities(%q)\n got: %v\nwant: %v", tc.mask, got, tc.want)
			}
		})
	}
}

func TestDecodeCapabilitiesFullRootSet(t *testing.T) {
	// Bits 0..40 — every capability up to CAP_LAST_CAP on Linux 5.9+.
	got := decodeCapabilities("000001ffffffffff")
	if len(got) != len(capabilityNames) {
		t.Fatalf("full mask decoded %d capabilities, want %d", len(got), len(capabilityNames))
	}
	if got[0] != "CAP_CHOWN" || got[len(got)-1] != "CAP_CHECKPOINT_RESTORE" {
		t.Fatalf("full mask bounds wrong: first=%s last=%s", got[0], got[len(got)-1])
	}
}
