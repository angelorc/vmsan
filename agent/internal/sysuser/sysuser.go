package sysuser

import (
	"fmt"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var validUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,32}$`)

// Credentials holds resolved system user identity.
type Credentials struct {
	Uid      uint32
	Gid      uint32
	HomeDir  string
	Username string
}

var cache sync.Map // map[string]*Credentials

// Resolve validates and looks up a system user, returning cached credentials
// on subsequent calls for the same username.
func Resolve(username string) (*Credentials, error) {
	if v, ok := cache.Load(username); ok {
		return v.(*Credentials), nil
	}
	if !validUsernameRe.MatchString(username) {
		return nil, fmt.Errorf("invalid username %q", username)
	}
	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("unknown user %q", username)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse gid %q: %w", u.Gid, err)
	}
	creds := &Credentials{
		Uid:      uint32(uid),
		Gid:      uint32(gid),
		HomeDir:  u.HomeDir,
		Username: username,
	}
	cache.Store(username, creds)
	return creds, nil
}

// Apply sets the process credentials, working directory (if empty), and
// replaces HOME/USER/LOGNAME in cmd.Env (filtering out any existing entries
// before appending the new values).
func (c *Credentials) Apply(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: c.Uid, Gid: c.Gid},
	}
	if cmd.Dir == "" {
		cmd.Dir = c.HomeDir
	}
	if cmd.Env == nil {
		cmd.Env = cmd.Environ()
	}
	// Filter out existing HOME/USER/LOGNAME entries.
	filtered := cmd.Env[:0]
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "USER=") || strings.HasPrefix(e, "LOGNAME=") {
			continue
		}
		filtered = append(filtered, e)
	}
	cmd.Env = append(filtered, "HOME="+c.HomeDir, "USER="+c.Username, "LOGNAME="+c.Username)
}
