package database

import "testing"

func TestEnvInt(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		def  int
		want int
	}{
		{"unset uses default", false, "", 100, 100},
		{"valid override", true, "200", 100, 200},
		{"empty uses default", true, "", 100, 100},
		{"non-numeric uses default", true, "lots", 100, 100},
		{"zero ignored (cannot disable ceiling)", true, "0", 100, 100},
		{"negative ignored", true, "-5", 100, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const key = "REDFLAG_DB_TEST_ENVINT"
			if c.set {
				t.Setenv(key, c.val)
			}
			if got := envInt(key, c.def); got != c.want {
				t.Errorf("envInt(%q, %d) = %d, want %d", c.val, c.def, got, c.want)
			}
		})
	}
}
