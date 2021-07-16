package cherrySession

import (
	"github.com/cherry-game/cherry/error"
	facade "github.com/cherry-game/cherry/facade"
	"github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/profile"
	"sync"
	"sync/atomic"
)

const (
	groupStatusWorking = 0
	groupStatusClosed  = 1
)

// SessionFilter represents a filter which was used to filter session when Multicast,
// the session will receive the message while filter returns true.
type SessionFilter func(*Session) bool

// Group represents a session group which used to manage a number of
// sessions, data send to the group will send to all session in it.
type Group struct {
	mu       sync.RWMutex
	status   int32                   // channel current status
	name     string                  // channel name
	sessions map[facade.SID]*Session // session id map to session instance
}

// NewGroup returns a new group instance
func NewGroup(n string) *Group {
	return &Group{
		status:   groupStatusWorking,
		name:     n,
		sessions: make(map[facade.SID]*Session),
	}
}

// Member returns specified UID's session
func (c *Group) Member(uid int64) (*Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, s := range c.sessions {
		if s.UID() == uid {
			return s, nil
		}
	}

	return nil, cherryError.SessionMemberNotFound
}

// Members returns all member's UID in current group
func (c *Group) Members() []int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var members []int64
	for _, s := range c.sessions {
		members = append(members, s.UID())
	}

	return members
}

// Multicast  push  the message to the filtered clients
func (c *Group) Multicast(route string, v interface{}, filter SessionFilter) error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	if cherryProfile.Debug() {
		cherryLogger.Debugf("multicast[%s], Data[%+v]", route, v)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, s := range c.sessions {
		if !filter(s) {
			continue
		}
		if err := s.Push(route, v); err != nil {
			s.Warn(err)
		}
	}

	return nil
}

// Broadcast push  the message(s) to  all members
func (c *Group) Broadcast(route string, v interface{}) error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	if cherryProfile.Debug() {
		cherryLogger.Debugf("broadcast[%s], data[%+v]", route, v)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, s := range c.sessions {
		if err := s.Push(route, v); err != nil {
			s.Warnf("push message error, SID[%d], UID[%d], Error[%s]", s.SID(), s.UID(), err.Error())
		}
	}

	return nil
}

// Contains check whether a UID is contained in current group or not
func (c *Group) Contains(uid int64) bool {
	_, err := c.Member(uid)
	return err == nil
}

// Add add session to group
func (c *Group) Add(session *Session) error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	if cherryProfile.Debug() {
		session.Debugf("add session to group[%s], SID[%d], UID[%d]", c.name, session.SID(), session.UID())
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	id := session.sid
	_, ok := c.sessions[session.sid]
	if ok {
		return cherryError.SessionDuplication
	}

	c.sessions[id] = session
	return nil
}

// Leave remove specified UID related session from group
func (c *Group) Leave(s *Session) error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	if cherryProfile.Debug() {
		s.Debugf("remove session from group[%s], UID[%d]", c.name, s.UID())
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.sessions, s.sid)
	return nil
}

// LeaveAll clear all sessions in the group
func (c *Group) LeaveAll() error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions = make(map[int64]*Session)
	return nil
}

// Count get current member amount in the group
func (c *Group) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.sessions)
}

func (c *Group) isClosed() bool {
	if atomic.LoadInt32(&c.status) == groupStatusClosed {
		return true
	}
	return false
}

// Close destroy group, which will release all resource in the group
func (c *Group) Close() error {
	if c.isClosed() {
		return cherryError.SessionClosedGroup
	}

	atomic.StoreInt32(&c.status, groupStatusClosed)

	// release all reference
	c.sessions = make(map[int64]*Session)
	return nil
}