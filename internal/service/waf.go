package service

import (
	"errors"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func (s *AppService) SaveWAF(id string, p domain.WAFInstance) (domain.WAFInstance, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.ID = id
	if id == "" {
		p.ID = domain.RandomID()
	}
	err := s.updateState(func(st *State) error {
		if id == "" {
			st.WAFInstances = append(st.WAFInstances, p)
		} else {
			found := false
			for i, v := range st.WAFInstances {
				if v.ID == id {
					st.WAFInstances[i] = p
					found = true
					break
				}
			}
			if !found {
				return errors.New("找不到防护实例")
			}
		}
		return domain.ValidateState(*st)
	})
	return p, err
}
func (s *AppService) DeleteWAF(id string) error {
	return s.updateState(func(st *State) error {
		for i, v := range st.WAFInstances {
			if v.ID == id {
				st.WAFInstances = append(st.WAFInstances[:i], st.WAFInstances[i+1:]...)
				return nil
			}
		}
		return errors.New("找不到防护实例")
	})
}
