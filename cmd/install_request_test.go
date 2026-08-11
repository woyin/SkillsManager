package cmd

import "testing"

func TestClassifyInstallRequest(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		fromRegistry bool
		fromLock     bool
		wantMode     installMode
		wantArg      string
		wantErr      bool
	}{
		{name: "profile without arguments", wantMode: profileInstallMode},
		{name: "bare name uses registry", args: []string{"my-skill"}, wantMode: registryInstallMode, wantArg: "my-skill"},
		{name: "repository uses direct install", args: []string{"owner/repository"}, wantMode: directInstallMode, wantArg: "owner/repository"},
		{name: "well-known source uses direct install", args: []string{"vercel-labs/skills"}, wantMode: directInstallMode, wantArg: "vercel-labs/skills"},
		{name: "deprecated registry flag wins", args: []string{"my-skill"}, fromRegistry: true, fromLock: true, wantMode: registryInstallMode, wantArg: "my-skill"},
		{name: "registry flag needs one argument", fromRegistry: true, wantErr: true},
		{name: "lock restore accepts command arguments", args: []string{"ignored"}, fromLock: true, wantMode: lockRestoreMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotArg, err := classifyInstallRequest(tt.args, tt.fromRegistry, tt.fromLock)
			if (err != nil) != tt.wantErr {
				t.Fatalf("classifyInstallRequest() error = %v, want error = %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotMode != tt.wantMode || gotArg != tt.wantArg {
				t.Fatalf("classifyInstallRequest() = (%v, %q), want (%v, %q)", gotMode, gotArg, tt.wantMode, tt.wantArg)
			}
		})
	}
}
