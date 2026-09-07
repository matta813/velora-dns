package filtering

import (
	"context"
	"testing"
)

type testStore struct {
	sources []Source
	id      int64
	err     error
}

func (s *testStore) LoadSources(_ context.Context) ([]Source, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sources, s.err
}
func (s *testStore) SaveSource(_ context.Context, source Source) (Source, error) {
	if s.err != nil {
		return Source{}, s.err
	}
	if source.ID == 0 {
		s.id++
		source.ID = s.id
		s.sources = append(s.sources, source)
	} else {
		for i := range s.sources {
			if s.sources[i].ID == source.ID {
				s.sources[i] = source
				break
			}
		}
	}
	return source, nil
}
func (s *testStore) DeleteSource(_ context.Context, id int64) error {
	if s.err != nil {
		return s.err
	}
	for i := range s.sources {
		if s.sources[i].ID == id {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func TestServiceCreateAndBlock(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example\ntracker.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		domain  string
		blocked bool
	}{{"ads.example", true}, {"a.ads.example", true}, {"tracker.example", true}, {"safe.example", false}} {
		if service.Blocked(tc.domain) != tc.blocked {
			t.Errorf("%s: blocked=%v", tc.domain, tc.blocked)
		}
	}
}

func TestServiceSetEnabled(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example")
	if err != nil {
		t.Fatal(err)
	}
	if !service.Blocked("ads.example") {
		t.Fatal("expected blocked")
	}
	_, err = service.SetEnabled(context.Background(), source.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if service.Blocked("ads.example") {
		t.Fatal("expected not blocked after disable")
	}
}

func TestServiceDelete(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(context.Background(), source.ID); err != nil {
		t.Fatal(err)
	}
	if service.Blocked("ads.example") {
		t.Fatal("expected not blocked after delete")
	}
}

func TestServiceDuplicateName(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "test", "", true)
	if err != ErrExists {
		t.Fatalf("expected ErrExists, got %v", err)
	}
}
