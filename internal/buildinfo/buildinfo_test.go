package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestVcsVersion_FullHash(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "815b174a3f9e2c1d"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	got := vcsVersion(info)
	if got != "815b174a" {
		t.Errorf("vcsVersion() = %q, want %q", got, "815b174a")
	}
}

func TestVcsVersion_Dirty(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "815b174a3f9e2c1d"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	got := vcsVersion(info)
	if got != "815b174a-dirty" {
		t.Errorf("vcsVersion() = %q, want %q", got, "815b174a-dirty")
	}
}

func TestVcsVersion_ShortHash(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc1234"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	got := vcsVersion(info)
	if got != "abc1234" {
		t.Errorf("vcsVersion() = %q, want %q", got, "abc1234")
	}
}

func TestVcsVersion_NoRevision(t *testing.T) {
	info := &debug.BuildInfo{}
	got := vcsVersion(info)
	if got != "dev" {
		t.Errorf("vcsVersion() = %q, want %q", got, "dev")
	}
}

func TestVcsVersion_ExactlyEightChars(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef12"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	got := vcsVersion(info)
	if got != "abcdef12" {
		t.Errorf("vcsVersion() = %q, want %q", got, "abcdef12")
	}
}

func TestVcsVersion_IgnoresUnrelatedSettings(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.time", Value: "2025-01-01T00:00:00Z"},
			{Key: "vcs.revision", Value: "deadbeef01234567"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	got := vcsVersion(info)
	if got != "deadbeef" {
		t.Errorf("vcsVersion() = %q, want %q", got, "deadbeef")
	}
}
