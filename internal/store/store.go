package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/darunshen/AIR/internal/model"
)

type Store struct {
	path string
}

type fileData struct {
	Sessions []*model.Session `json:"sessions"`
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		initial := fileData{Sessions: []*model.Session{}}
		if err := writeFile(path, &initial); err != nil {
			return nil, err
		}
	}

	return &Store{path: path}, nil
}

func (s *Store) Save(item *model.Session) error {
	data, err := s.read()
	if err != nil {
		return err
	}

	for i, existing := range data.Sessions {
		if existing.ID == item.ID {
			data.Sessions[i] = item
			return writeFile(s.path, data)
		}
	}

	data.Sessions = append(data.Sessions, item)
	return writeFile(s.path, data)
}

func (s *Store) Get(id string) (*model.Session, error) {
	data, err := s.read()
	if err != nil {
		return nil, err
	}

	for _, item := range data.Sessions {
		if item.ID == id {
			return item, nil
		}
	}

	return nil, errors.New("session not found")
}

func (s *Store) Delete(id string) error {
	data, err := s.read()
	if err != nil {
		return err
	}

	next := make([]*model.Session, 0, len(data.Sessions))
	found := false
	for _, item := range data.Sessions {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return errors.New("session not found")
	}

	data.Sessions = next
	return writeFile(s.path, data)
}

func (s *Store) read() (*fileData, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}

	var data fileData
	if len(raw) == 0 {
		data.Sessions = []*model.Session{}
		return &data, nil
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if data.Sessions == nil {
		data.Sessions = []*model.Session{}
	}
	return &data, nil
}

func writeFile(path string, data *fileData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
