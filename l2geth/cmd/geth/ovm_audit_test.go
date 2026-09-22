package main

import "testing"

func TestOVMAuditFlags(t *testing.T) {
	hash := "0x31ccdca9cb607cdde996f7a8a82519bc873ffff68b898a3d8472069fc5724226"
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"implicit-fast", nil, "OVM audit requires explicit --syncmode full"},
		{"fast", []string{"--syncmode", "fast"}, "OVM audit requires explicit --syncmode full"},
		{"light", []string{"--syncmode", "light"}, "OVM audit requires explicit --syncmode full"},
		{"missing-target", []string{"--syncmode", "full"}, "OVM audit requires --ovm.audit.to and --ovm.audit.to-hash"},
		{"bad-hash", []string{"--syncmode", "full", "--ovm.audit.to-hash", "0x01"}, "Invalid ovm.audit.to-hash"},
		{"mining", []string{"--syncmode", "full", "--ovm.audit.to", "1", "--ovm.audit.to-hash", hash, "--mine"}, "OVM audit cannot be combined"},
		{"dev", []string{"--syncmode", "full", "--ovm.audit.to", "1", "--ovm.audit.to-hash", hash, "--dev"}, "OVM audit cannot be combined"},
		{"exitwhensynced", []string{"--syncmode", "full", "--ovm.audit.to", "1", "--ovm.audit.to-hash", hash, "--exitwhensynced"}, "OVM audit cannot be combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--datadir", t.TempDir(), "--ovm.audit", "--maxpeers", "0", "--nodiscover"}, tc.args...)
			geth := runGeth(t, args...)
			geth.ExpectRegexp(tc.message + "[^\r\n]*\r?\n")
			geth.ExpectExit()
			if geth.ExitStatus() == 0 {
				t.Fatal("invalid audit flags exited successfully")
			}
		})
	}
}
