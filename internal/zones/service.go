package zones

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	wire "github.com/miekg/dns"
)

type Service struct {
	repo       Repository
	mu         sync.Mutex
	state      atomic.Pointer[snapshot]
	invalidate func()
}

func New(ctx context.Context, repo Repository, invalidate func()) (*Service, error) {
	all, err := repo.LoadZones(ctx)
	if err != nil {
		return nil, err
	}
	state, err := compile(all)
	if err != nil {
		return nil, err
	}
	s := &Service{repo: repo, invalidate: invalidate}
	s.state.Store(state)
	return s, nil
}
func (s *Service) List() []Zone {
	all := []Zone{}
	for _, z := range s.state.Load().byID {
		all = append(all, clone(z))
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}
func (s *Service) Get(id int64) (Zone, error) {
	z, ok := s.state.Load().byID[id]
	if !ok {
		return Zone{}, ErrNotFound
	}
	return clone(z), nil
}
func (s *Service) Create(ctx context.Context, z Zone) (Zone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if z.ID != 0 || z.Revision != 0 {
		return Zone{}, invalid("IDs and revision are assigned by the server")
	}
	for _, r := range z.Records {
		if r.ID != 0 {
			return Zone{}, invalid("new records cannot supply IDs")
		}
	}
	z.Revision = 1
	return s.save(ctx, z, 0)
}
func (s *Service) Update(ctx context.Context, id int64, revision uint32, z Zone) (Zone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.state.Load().byID[id]
	if !ok {
		return Zone{}, ErrNotFound
	}
	if old.Revision != revision || revision == math.MaxUint32 {
		return Zone{}, ErrConflict
	}
	ids := map[int64]bool{}
	for _, r := range old.Records {
		ids[r.ID] = true
	}
	for _, r := range z.Records {
		if r.ID != 0 && !ids[r.ID] {
			return Zone{}, invalid("record ID does not belong to this zone")
		}
	}
	z.ID = id
	z.Revision = revision + 1
	return s.save(ctx, z, revision)
}
func (s *Service) save(ctx context.Context, z Zone, expected uint32) (Zone, error) {
	all := s.List()
	for i, old := range all {
		if old.ID == z.ID {
			all = append(all[:i], all[i+1:]...)
			break
		}
	}
	all = append(all, z)
	next, err := compile(all)
	if err != nil {
		return Zone{}, err
	}
	saved, err := s.repo.SaveZone(ctx, next.byID[z.ID], expected)
	if err != nil {
		return Zone{}, err
	}
	delete(next.byID, z.ID)
	next.byID[saved.ID] = clone(saved)
	next.byName[saved.Name].zone = clone(saved)
	s.state.Store(next)
	if s.invalidate != nil {
		s.invalidate()
	}
	return clone(saved), nil
}
func (s *Service) Delete(ctx context.Context, id int64, revision uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	z, ok := s.state.Load().byID[id]
	if !ok {
		return ErrNotFound
	}
	if z.Revision != revision {
		return ErrConflict
	}
	all := s.List()
	for i, z := range all {
		if z.ID == id {
			all = append(all[:i], all[i+1:]...)
			break
		}
	}
	next, err := compile(all)
	if err != nil {
		return err
	}
	if err = s.repo.DeleteZone(ctx, id, revision); err != nil {
		return err
	}
	s.state.Store(next)
	if s.invalidate != nil {
		s.invalidate()
	}
	return nil
}

// Lookup answers from one consistent snapshot; local misses never leak upstream.
func (s *Service) Lookup(q *wire.Msg) (*wire.Msg, bool) {
	if len(q.Question) != 1 || q.Question[0].Qclass != wire.ClassINET {
		return nil, false
	}
	state := s.state.Load()
	name := strings.ToLower(q.Question[0].Name)
	owner := state.find(name)
	if owner == nil {
		return nil, false
	}
	m := new(wire.Msg)
	m.SetReply(q)
	m.Authoritative = true
	m.RecursionAvailable = true
	kind := q.Question[0].Qtype
	for depth := 0; depth < 16; depth++ {
		owner = state.find(name)
		if owner == nil {
			return m, true
		}
		if answer := owner.lookup(name, kind); len(answer) > 0 {
			m.Answer = append(m.Answer, answer...)
			return m, true
		}
		if cname := owner.lookup(name, wire.TypeCNAME); len(cname) > 0 {
			m.Answer = append(m.Answer, cname...)
			name = cname[0].(*wire.CNAME).Target
			continue
		}
		if !owner.exists[name] {
			m.Rcode = wire.RcodeNameError
		}
		m.Ns = []wire.RR{owner.soa()}
		return m, true
	}
	m.Answer = nil
	m.Rcode = wire.RcodeServerFailure
	return m, true
}
