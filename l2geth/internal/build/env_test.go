package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetBuildEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GIT_COMMIT", "GIT_DATE", "CI", "TRAVIS", "APPVEYOR", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		t.Setenv(key, "")
	}
	// An empty GIT_DIR is still an explicit (invalid) repository location.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []*string{GitCommitFlag, GitDateFlag, GitBranchFlag, GitTagFlag, BuildnumFlag} {
		old := *value
		*value = ""
		t.Cleanup(func() { *value = old })
	}
	for _, value := range []*bool{PullRequestFlag, CronJobFlag} {
		old := *value
		*value = false
		t.Cleanup(func() { *value = old })
	}
}

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2020-01-02T00:30:00Z", "GIT_COMMITTER_DATE=2020-01-02T00:30:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func testRepository(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required for repository discovery tests")
	}
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "metadata-test")
	testGit(t, dir, "-c", "user.name=Build Test", "-c", "user.email=build@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "build metadata")
	testGit(t, dir, "tag", "metadata-tag")
	return dir, testGit(t, dir, "rev-parse", "HEAD")
}

func TestLocalEnvRepositoryLayouts(t *testing.T) {
	for _, layout := range []string{"root", "subdirectory", "packed-refs", "detached", "worktree"} {
		t.Run(layout, func(t *testing.T) {
			resetBuildEnv(t)
			dir, commit := testRepository(t)
			branch := "metadata-test"
			switch layout {
			case "subdirectory":
				dir = filepath.Join(dir, "l2geth")
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "packed-refs":
				testGit(t, dir, "pack-refs", "--all", "--prune")
			case "detached":
				testGit(t, dir, "checkout", "--detach", "HEAD")
				branch = ""
			case "worktree":
				worktree := filepath.Join(t.TempDir(), "checkout")
				testGit(t, dir, "worktree", "add", "-b", "linked", worktree, "HEAD")
				dir, branch = worktree, "linked"
			}
			t.Chdir(dir)
			// The UTC date must not depend on the build host's local timezone.
			oldLocal := time.Local
			time.Local = time.FixedZone("test-west", -8*60*60)
			t.Cleanup(func() { time.Local = oldLocal })
			env := LocalEnv()
			if env.Commit != commit || env.Date != "20200102" || env.Branch != branch || env.Tag != "metadata-tag" {
				t.Fatalf("unexpected metadata: %s", env)
			}
		})
	}
}

func TestLocalEnvOverrides(t *testing.T) {
	resetBuildEnv(t)
	dir, _ := testRepository(t)
	t.Chdir(dir)
	// Deliberately use a hash absent from the repository: a supplied date must
	// prevent Git from being queried for that commit's date.
	t.Setenv("GIT_COMMIT", strings.Repeat("a", 40))
	t.Setenv("GIT_DATE", "20240203")
	if env := LocalEnv(); env.Commit != strings.Repeat("a", 40) || env.Date != "20240203" {
		t.Fatalf("environment overrides lost: %s", env)
	}
	*GitCommitFlag, *GitDateFlag = strings.Repeat("b", 40), "20250304"
	*GitBranchFlag, *GitTagFlag = "explicit-branch", "explicit-tag"
	if env := LocalEnv(); env.Commit != *GitCommitFlag || env.Date != *GitDateFlag || env.Branch != *GitBranchFlag || env.Tag != *GitTagFlag {
		t.Fatalf("flag overrides lost: %s", env)
	}
}

func TestLocalEnvWithoutGit(t *testing.T) {
	for _, missingExecutable := range []bool{false, true} {
		t.Run(map[bool]string{false: "source-archive", true: "missing-git"}[missingExecutable], func(t *testing.T) {
			resetBuildEnv(t)
			t.Chdir(t.TempDir())
			if missingExecutable {
				t.Setenv("PATH", "")
			}
			if env := LocalEnv(); env.Commit != "" || env.Date != "" || env.Branch != "" || env.Tag != "" {
				t.Fatalf("unexpected metadata without Git: %s", env)
			}
			t.Setenv("GIT_COMMIT", strings.Repeat("a", 40))
			t.Setenv("GIT_DATE", "20240203")
			if env := LocalEnv(); env.Commit != strings.Repeat("a", 40) || env.Date != "20240203" {
				t.Fatalf("archive metadata lost: %s", env)
			}
		})
	}
}

func TestEnvCIOverrides(t *testing.T) {
	for _, ci := range []string{"travis", "appveyor"} {
		t.Run(ci, func(t *testing.T) {
			resetBuildEnv(t)
			dir, commit := testRepository(t)
			t.Chdir(dir)
			if ci == "travis" {
				t.Setenv("CI", "true")
				t.Setenv("TRAVIS", "true")
				t.Setenv("TRAVIS_PULL_REQUEST_SHA", "")
				t.Setenv("TRAVIS_COMMIT", commit)
			} else {
				t.Setenv("CI", "True")
				t.Setenv("APPVEYOR", "True")
				t.Setenv("APPVEYOR_PULL_REQUEST_HEAD_COMMIT", "")
				t.Setenv("APPVEYOR_REPO_COMMIT", commit)
			}
			if env := Env(); env.Name != ci || env.Commit != commit || env.Date != "20200102" {
				t.Fatalf("CI metadata lost: %s", env)
			}
			t.Chdir(t.TempDir())
			t.Setenv("GIT_COMMIT", strings.Repeat("a", 40))
			t.Setenv("GIT_DATE", "20240203")
			if env := Env(); env.Name != ci || env.Commit != strings.Repeat("a", 40) || env.Date != "20240203" {
				t.Fatalf("CI environment overrides lost: %s", env)
			}
			*GitCommitFlag, *GitDateFlag = strings.Repeat("b", 40), "20250304"
			if env := Env(); env.Commit != *GitCommitFlag || env.Date != *GitDateFlag {
				t.Fatalf("CI flag overrides lost: %s", env)
			}
		})
	}
}
