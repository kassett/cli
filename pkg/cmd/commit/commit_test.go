package commit

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       commitOptions
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "no argument",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"branch\", \"message\" not set",
		},
		{
			name:     "match multiple patterns",
			cli:      "-m \"commit message\" -b main file1 file2",
			wantsErr: false,
			wants: commitOptions{
				CommitMessage:  "commit message",
				PatternMatches: []string{"file1", "file2"},
				Branch:         "main",
			},
		},
		{
			name:     "use commit all",
			cli:      "-m \"commit message\" -b main --all",
			wantsErr: false,
			wants: commitOptions{
				CommitMessage: "commit message",
				Branch:        "main",
				CommitAll:     true,
			},
		},
		{
			name:        "fail on commit all and force",
			cli:         "-m \"commit message\" -b main --all --force",
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--all`, `--force`",
		},
		{
			name: "test dry run",
			cli:  "-m \"commit message\" -b main --dry-run",
			wants: commitOptions{
				CommitMessage: "commit message",
				Branch:        "main",
				DryRun:        true,
			},
			wantsErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts commitOptions
			cmd := NewCmdCommit(f, func(config *commitOptions) error {
				gotOpts = *config
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Branch, gotOpts.Branch)
			assert.Equal(t, tt.wants.CommitMessage, gotOpts.CommitMessage)
			assert.Equal(t, tt.wants.CommitAll, gotOpts.CommitAll)
			assert.Equal(t, tt.wants.Force, gotOpts.Force)
			assert.Equal(t, tt.wants.SyncWithRemote, gotOpts.SyncWithRemote)
			assert.Equal(t, tt.wants.DryRun, gotOpts.DryRun)
		})
	}
}
