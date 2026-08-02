//go:build !windows && !darwin

package main

func SetAutoStart(bool) error {
	return nil
}
