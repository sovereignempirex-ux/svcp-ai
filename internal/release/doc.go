// Package release holds nothing that ships. It exists so the files that describe
// a release can be checked against each other.
//
// A release is described in three places that never see each other at build time:
// the goreleaser configuration decides what is produced, the install script
// decides what it will go looking for, and scripts/publish-release.ps1 decides
// what is attached. Nothing in the toolchain compares them, so a rename in one
// is discovered by a user on a platform that nobody tested, as a 404. The checks
// live here so that cannot happen quietly.
package release
