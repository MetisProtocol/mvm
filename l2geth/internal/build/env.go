// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package build

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	// These flags override values in build env.
	GitCommitFlag   = flag.String("git-commit", "", `Overrides git commit hash embedded into executables`)
	GitDateFlag     = flag.String("git-date", "", `Overrides git commit date embedded into executables (YYYYMMDD)`)
	GitBranchFlag   = flag.String("git-branch", "", `Overrides git branch being built`)
	GitTagFlag      = flag.String("git-tag", "", `Overrides git tag being built`)
	BuildnumFlag    = flag.String("buildnum", "", `Overrides CI build number`)
	PullRequestFlag = flag.Bool("pull-request", false, `Overrides pull request status of the build`)
	CronJobFlag     = flag.Bool("cron-job", false, `Overrides cron job status of the build`)
)

// Environment contains metadata provided by the build environment.
type Environment struct {
	Name                      string // name of the environment
	Repo                      string // name of GitHub repo
	Commit, Date, Branch, Tag string // Git info
	Buildnum                  string
	IsPullRequest             bool
	IsCronJob                 bool
}

func (env Environment) String() string {
	return fmt.Sprintf("%s env (commit:%s date:%s branch:%s tag:%s buildnum:%s pr:%t)",
		env.Name, env.Commit, env.Date, env.Branch, env.Tag, env.Buildnum, env.IsPullRequest)
}

// Env returns metadata about the current CI environment, falling back to LocalEnv
// if not running on CI.
func Env() Environment {
	var env Environment
	switch {
	case os.Getenv("CI") == "true" && os.Getenv("TRAVIS") == "true":
		commit := os.Getenv("TRAVIS_PULL_REQUEST_SHA")
		if commit == "" {
			commit = os.Getenv("TRAVIS_COMMIT")
		}
		env = Environment{
			Name:          "travis",
			Repo:          os.Getenv("TRAVIS_REPO_SLUG"),
			Commit:        commit,
			Branch:        os.Getenv("TRAVIS_BRANCH"),
			Tag:           os.Getenv("TRAVIS_TAG"),
			Buildnum:      os.Getenv("TRAVIS_BUILD_NUMBER"),
			IsPullRequest: os.Getenv("TRAVIS_PULL_REQUEST") != "false",
			IsCronJob:     os.Getenv("TRAVIS_EVENT_TYPE") == "cron",
		}
	case os.Getenv("CI") == "True" && os.Getenv("APPVEYOR") == "True":
		commit := os.Getenv("APPVEYOR_PULL_REQUEST_HEAD_COMMIT")
		if commit == "" {
			commit = os.Getenv("APPVEYOR_REPO_COMMIT")
		}
		env = Environment{
			Name:          "appveyor",
			Repo:          os.Getenv("APPVEYOR_REPO_NAME"),
			Commit:        commit,
			Branch:        os.Getenv("APPVEYOR_REPO_BRANCH"),
			Tag:           os.Getenv("APPVEYOR_REPO_TAG_NAME"),
			Buildnum:      os.Getenv("APPVEYOR_BUILD_NUMBER"),
			IsPullRequest: os.Getenv("APPVEYOR_PULL_REQUEST_NUMBER") != "",
			IsCronJob:     os.Getenv("APPVEYOR_SCHEDULED_BUILD") == "True",
		}
	default:
		return LocalEnv()
	}
	env = applyEnvFlags(env)
	if env.Date == "" && gitHead() != "" {
		env.Date = getDate(env.Commit)
	}
	return env
}

// LocalEnv returns build environment metadata gathered from git.
func LocalEnv() Environment {
	env := applyEnvFlags(Environment{Name: "local", Repo: "ethereum/go-ethereum"})

	head := gitHead()
	if head == "" {
		return env
	}
	if env.Commit == "" {
		env.Commit = head
	}
	if env.Date == "" {
		env.Date = getDate(env.Commit)
	}
	if env.Branch == "" {
		if branch := RunGit("rev-parse", "--abbrev-ref", "HEAD"); branch != "HEAD" {
			env.Branch = branch
		}
	}
	if env.Tag == "" {
		env.Tag = firstLine(RunGit("tag", "-l", "--points-at", "HEAD"))
	}
	return env
}

// gitHead lets Git discover the repository, including parent directories and
// worktrees. Source archives and environments without Git have no local metadata.
func gitHead() string {
	out, err := exec.Command("git", "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstLine(s string) string {
	return strings.Split(s, "\n")[0]
}

func getDate(commit string) string {
	if commit == "" {
		return ""
	}
	out := RunGit("show", "-s", "--format=%ct", commit)
	if out == "" {
		return ""
	}
	date, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse git commit date: %v", err))
	}
	return time.Unix(date, 0).UTC().Format("20060102")
}

func applyEnvFlags(env Environment) Environment {
	if !flag.Parsed() {
		panic("you need to call flag.Parse before Env or LocalEnv")
	}
	if commit := os.Getenv("GIT_COMMIT"); commit != "" {
		env.Commit = commit
	}
	if date := os.Getenv("GIT_DATE"); date != "" {
		env.Date = date
	}
	if *GitCommitFlag != "" {
		env.Commit = *GitCommitFlag
	}
	if *GitDateFlag != "" {
		env.Date = *GitDateFlag
	}
	if *GitBranchFlag != "" {
		env.Branch = *GitBranchFlag
	}
	if *GitTagFlag != "" {
		env.Tag = *GitTagFlag
	}
	if *BuildnumFlag != "" {
		env.Buildnum = *BuildnumFlag
	}
	if *PullRequestFlag {
		env.IsPullRequest = true
	}
	if *CronJobFlag {
		env.IsCronJob = true
	}
	return env
}
