package permissions

type Checker struct {
	Root    string
	Session *SessionStore
	Project []Rule
	User    []Rule
}

func (c Checker) Check(request Request) Decision {
	if request.Root == "" {
		request.Root = c.Root
	}
	if decision, blocked := CheckDangerousCommand(request); blocked {
		return decision
	}
	if decision, blocked := CheckSandbox(request); blocked {
		return decision
	}
	var session []Rule
	if c.Session != nil {
		session = c.Session.Rules()
	}
	return DecideByRules(request, session, c.Project, c.User)
}

func (c Checker) AddSessionRule(rule Rule) {
	if c.Session != nil {
		c.Session.Add(rule)
	}
}
