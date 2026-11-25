package main

import (
	"os"
	"testing"
)

func TestLoadBootstrapPasswordReadsAndUnsetsEnvironmentValue(t *testing.T) {
	t.Setenv("BOOTSTRAP_PASSWORD", "a secure password")
	readFromTerminalCalled := false

	password, err := loadBootstrapPassword(func() ([]byte, error) {
		readFromTerminalCalled = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("loadBootstrapPassword() error = %v", err)
	}
	defer clear(password)

	if string(password) != "a secure password" {
		t.Fatalf("password = %q, want environment value", password)
	}
	if readFromTerminalCalled {
		t.Fatal("terminal prompt was called when an environment value was set")
	}
	if _, ok := os.LookupEnv("BOOTSTRAP_PASSWORD"); ok {
		t.Fatal("BOOTSTRAP_PASSWORD remains set")
	}
}

func TestLoadBootstrapPasswordPromptsWhenEnvironmentIsUnset(t *testing.T) {
	previous, wasSet := os.LookupEnv("BOOTSTRAP_PASSWORD")
	if err := os.Unsetenv("BOOTSTRAP_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv("BOOTSTRAP_PASSWORD", previous)
		} else {
			_ = os.Unsetenv("BOOTSTRAP_PASSWORD")
		}
	})

	readFromTerminalCalled := false
	password, err := loadBootstrapPassword(func() ([]byte, error) {
		readFromTerminalCalled = true
		return []byte("a secure password"), nil
	})
	if err != nil {
		t.Fatalf("loadBootstrapPassword() error = %v", err)
	}
	defer clear(password)

	if string(password) != "a secure password" {
		t.Fatalf("password = %q, want terminal value", password)
	}
	if !readFromTerminalCalled {
		t.Fatal("terminal prompt was not called")
	}
}

func TestLoadBootstrapPasswordRejectsShortPassword(t *testing.T) {
	previous, wasSet := os.LookupEnv("BOOTSTRAP_PASSWORD")
	if err := os.Unsetenv("BOOTSTRAP_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv("BOOTSTRAP_PASSWORD", previous)
		} else {
			_ = os.Unsetenv("BOOTSTRAP_PASSWORD")
		}
	})

	password, err := loadBootstrapPassword(func() ([]byte, error) {
		return []byte("short"), nil
	})
	if err == nil {
		clear(password)
		t.Fatal("loadBootstrapPassword() error = nil, want length error")
	}
}
