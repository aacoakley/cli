package archive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/prompt"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ArchiveOptions struct {
	HttpClient      func() (*http.Client, error)
	IO              *iostreams.IOStreams
	Config          func() (config.Config, error)
	BaseRepo        func() (ghrepo.Interface, error)
	RepoArg         string
	HasRepoOverride bool
	Confirmed       bool
}

func NewCmdArchive(f *cmdutil.Factory, runF func(*ArchiveOptions) error) *cobra.Command {
	opts := &ArchiveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive a repository",
		Long: heredoc.Doc(`Archive a GitHub repository.

With no argument, archives the current repository.`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if !opts.Confirmed && !opts.IO.CanPrompt() && !opts.HasRepoOverride {
				return cmdutil.FlagErrorf("could not prompt: confirmation with prompt or --confirm flag required")
			}

			if runF != nil {
				return runF(opts)
			}
			return archiveRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)
	cmd.Flags().BoolVarP(&opts.Confirmed, "confirm", "y", false, "skip confirmation prompt")
	return cmd
}

func archiveRun(opts *ArchiveOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toArchive ghrepo.Interface

	repoSelector := opts.RepoArg
	if repoSelector == "" {
		currRepo, err := opts.BaseRepo()
		if err != nil {
			return err
		}
		repoSelector = ghrepo.FullName(currRepo)
	}

	if !strings.Contains(repoSelector, "/") {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		hostname, err := cfg.DefaultHost()
		if err != nil {
			return err
		}

		currentUser, err := api.CurrentLoginName(apiClient, hostname)
		if err != nil {
			return err
		}
		repoSelector = currentUser + "/" + repoSelector
	}
	toArchive, err = ghrepo.FromFullName(repoSelector)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	fields := []string{"name", "owner", "isArchived", "id"}
	repo, err := api.FetchRepository(apiClient, toArchive, fields)
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toArchive)
	if repo.IsArchived {
		fmt.Fprintf(opts.IO.ErrOut,
			"%s Repository %s is already archived\n",
			cs.WarningIcon(),
			fullName)
		return nil
	}

	if !opts.Confirmed {
		p := &survey.Confirm{
			Message: fmt.Sprintf("Archive %s?", fullName),
			Default: false,
		}
		err = prompt.SurveyAskOne(p, &opts.Confirmed)
		if err != nil {
			return fmt.Errorf("failed to prompt: %w", err)
		}
		if !opts.Confirmed {
			return nil
		}
	}

	err = archiveRepo(httpClient, repo)
	if err != nil {
		return fmt.Errorf("API called failed: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out,
			"%s Archived repository %s\n",
			cs.SuccessIcon(),
			fullName)
	}

	return nil
}
