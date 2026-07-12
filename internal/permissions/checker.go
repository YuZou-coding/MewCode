package permissions

type Checker struct {
	Root        string
	Mode        Mode
	DefaultMode Mode
	Session     *SessionStore
	Project     []Rule
	User        []Rule
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
	decision := DecideByRules(request, session, c.Project, c.User)
	mode := c.CurrentMode()
	if decision.Effect == EffectDeny {
		decision.Mode = mode
		return decision
	}
	switch mode {
	case ModeStrict:
		return Decision{Effect: EffectAsk, Reason: "strict mode requires approval", Mode: mode}
	case ModeYOLO:
		return Decision{Effect: EffectAllow, Reason: "yolo mode", Mode: mode}
	default:
		decision.Mode = mode
		return decision
	}
}

func (c Checker) AddSessionRule(rule Rule) {
	if c.Session != nil {
		c.Session.Add(rule)
	}
}

func (c *Checker) CurrentMode() Mode {
	if c == nil {
		return ModeDefault
	}
	if mode, ok := ParseMode(string(c.Mode)); ok {
		return mode
	}
	return ModeDefault
}

func (c *Checker) SetMode(mode Mode) bool {
	if _, ok := ParseMode(string(mode)); !ok {
		return false
	}
	c.Mode = mode
	return true
}

func (c *Checker) ResetMode() {
	if c == nil {
		return
	}
	if mode, ok := ParseMode(string(c.DefaultMode)); ok {
		c.Mode = mode
		return
	}
	c.Mode = ModeDefault
}

func (c *Checker) InitialMode() Mode {
	if c == nil {
		return ModeDefault
	}
	if mode, ok := ParseMode(string(c.DefaultMode)); ok {
		return mode
	}
	return ModeDefault
}
