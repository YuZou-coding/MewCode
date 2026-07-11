package permissions

type SessionStore struct {
	rules []Rule
}

func NewSessionStore() *SessionStore {
	return &SessionStore{}
}

func (s *SessionStore) Add(rule Rule) {
	rule.Source = SourceSession
	s.rules = append(s.rules, rule)
}

func (s *SessionStore) Rules() []Rule {
	rules := make([]Rule, len(s.rules))
	copy(rules, s.rules)
	return rules
}

func (s *SessionStore) Clear() {
	s.rules = nil
}
