// Package sshx resolves ~/.ssh/config (HostName, User, Port, IdentityFile,
// ProxyJump), authenticates via ssh-agent with identity-file fallback, and
// verifies host keys against known_hosts. Implemented in M4.
package sshx
