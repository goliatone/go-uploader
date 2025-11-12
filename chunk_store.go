package uploader

import (
	"sync"
	"time"

	gerrors "github.com/goliatone/go-errors"
)

// ChunkSessionState represents the lifecycle stage of a chunked upload session.
type ChunkSessionState string

const (
	// ChunkSessionStateActive indicates chunks may still be uploaded.
	ChunkSessionStateActive ChunkSessionState = "active"
	// ChunkSessionStateCompleted is set after the finalization step succeeds.
	ChunkSessionStateCompleted ChunkSessionState = "completed"
	// ChunkSessionStateAborted is set when the session is canceled by the client or due to errors.
	ChunkSessionStateAborted ChunkSessionState = "aborted"
)

// ChunkPart captures metadata for an uploaded chunk.
type ChunkPart struct {
	Index      int
	Size       int64
	Checksum   string
	ETag       string
	UploadedAt time.Time
}

// ChunkSession keeps track of multipart upload progress and provider-specific details.
type ChunkSession struct {
	ID            string
	Key           string
	TotalSize     int64
	PartSize      int64
	Metadata      *Metadata
	CreatedAt     time.Time
	ExpiresAt     time.Time
	State         ChunkSessionState
	UploadedParts map[int]ChunkPart
	ProviderData  map[string]any
}

// ChunkSessionStore is an in-memory registry backed by a RWMutex. Implementation can be swapped later.
type ChunkSessionStore struct {
	mu        sync.RWMutex
	ttl       time.Duration
	sessions  map[string]*ChunkSession
	timeNowFn func() time.Time
}

// NewChunkSessionStore creates a new store with the provided TTL (or DefaultChunkSessionTTL if <= 0).
func NewChunkSessionStore(ttl time.Duration) *ChunkSessionStore {
	if ttl <= 0 {
		ttl = DefaultChunkSessionTTL
	}

	return &ChunkSessionStore{
		ttl:      ttl,
		sessions: make(map[string]*ChunkSession),
		timeNowFn: func() time.Time {
			return time.Now()
		},
	}
}

// timeNow returns the injectable clock function to simplify testing.
func (s *ChunkSessionStore) timeNow() time.Time {
	if s.timeNowFn != nil {
		return s.timeNowFn()
	}
	return time.Now()
}

// Create registers a new chunk upload session.
func (s *ChunkSessionStore) Create(session *ChunkSession) (*ChunkSession, error) {
	if session == nil {
		return nil, gerrors.NewValidation("chunk session definition required",
			gerrors.FieldError{
				Field:   "session",
				Message: "cannot be nil",
			},
		)
	}

	if session.ID == "" {
		return nil, gerrors.NewValidation("chunk session definition invalid",
			gerrors.FieldError{
				Field:   "id",
				Message: "cannot be empty",
			},
		)
	}

	if session.Key == "" {
		return nil, gerrors.NewValidation("chunk session definition invalid",
			gerrors.FieldError{
				Field:   "key",
				Message: "cannot be empty",
			},
		)
	}

	now := s.timeNow()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = session.CreatedAt.Add(s.ttl)
	}

	if session.UploadedParts == nil {
		session.UploadedParts = make(map[int]ChunkPart)
	}
	if session.ProviderData == nil {
		session.ProviderData = make(map[string]any)
	}
	if session.State == "" {
		session.State = ChunkSessionStateActive
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return nil, ErrChunkSessionExists
	}

	stored := cloneChunkSession(session)
	s.sessions[session.ID] = stored

	return cloneChunkSession(stored), nil
}

// Get returns a copy of the session if it exists and has not expired.
func (s *ChunkSessionStore) Get(id string) (*ChunkSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}

	if s.timeNow().After(session.ExpiresAt) {
		return nil, false
	}

	return cloneChunkSession(session), true
}

// Delete removes a session from the store.
func (s *ChunkSessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// AddPart registers a chunk part for the given session ID.
func (s *ChunkSessionStore) AddPart(id string, part ChunkPart) (*ChunkSession, error) {
	if part.Index < 0 {
		return nil, ErrChunkPartOutOfRange
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrChunkSessionNotFound
	}

	if s.timeNow().After(session.ExpiresAt) {
		delete(s.sessions, id)
		return nil, ErrChunkSessionNotFound
	}

	if session.State != ChunkSessionStateActive {
		return nil, ErrChunkSessionClosed
	}

	if _, exists := session.UploadedParts[part.Index]; exists {
		return nil, ErrChunkPartDuplicate
	}

	if part.UploadedAt.IsZero() {
		part.UploadedAt = s.timeNow()
	}

	session.UploadedParts[part.Index] = part

	return cloneChunkSession(session), nil
}

// MarkCompleted flags a session as completed if it is active.
func (s *ChunkSessionStore) MarkCompleted(id string) (*ChunkSession, error) {
	return s.updateState(id, ChunkSessionStateCompleted)
}

// MarkAborted flags a session as aborted if it is active.
func (s *ChunkSessionStore) MarkAborted(id string) (*ChunkSession, error) {
	return s.updateState(id, ChunkSessionStateAborted)
}

func (s *ChunkSessionStore) updateState(id string, newState ChunkSessionState) (*ChunkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrChunkSessionNotFound
	}

	if session.State != ChunkSessionStateActive {
		return nil, ErrChunkSessionClosed
	}

	session.State = newState
	return cloneChunkSession(session), nil
}

// CleanupExpired removes expired sessions and returns their IDs.
func (s *ChunkSessionStore) CleanupExpired(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var removed []string
	for id, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, id)
			removed = append(removed, id)
		}
	}

	return removed
}

func cloneChunkSession(in *ChunkSession) *ChunkSession {
	if in == nil {
		return nil
	}

	out := *in
	if in.Metadata != nil {
		metaCopy := *in.Metadata
		out.Metadata = &metaCopy
	}
	if len(in.UploadedParts) > 0 {
		out.UploadedParts = make(map[int]ChunkPart, len(in.UploadedParts))
		for idx, part := range in.UploadedParts {
			out.UploadedParts[idx] = part
		}
	}

	if len(in.ProviderData) > 0 {
		out.ProviderData = make(map[string]any, len(in.ProviderData))
		for k, v := range in.ProviderData {
			out.ProviderData[k] = v
		}
	}

	return &out
}
