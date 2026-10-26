package app

import "github.com/egoist/mygo/ui"

// agentButton chooses the agent Ship uses, its icon alone as a menu
// button.
func (a *app) agentButton(c *ui.Context) {
	def := agentDefOf(a.agentKind())
	t := c.Theme()
	label := "Ship with " + def.name
	tooltip := label
	if a.push.running {
		tooltip = "Shipping with " + def.name
	}
	b := ui.Button(c.Key("agent"), "").Shrink(0).Label(label).Tooltip(tooltip).
		Disabled(a.push.running)
	b.Menu(func(m *ui.Menu) {
		for _, d := range agentDefs {
			if m.Item(d.name).Checked(d.kind == a.agentKind()).Chosen() {
				a.chooseAgent(d.kind)
			}
		}
	})
	b.Children(func() {
		ui.Icon(c, def.icon).Size(16, 16)
		ui.Icon(c, chevronIcon).Size(8, 8).TextColor(t.TextMuted)
	})
}

// toggleAgent chooses the other agent, as the toolbar's agent button does
// where AppKit's toolbar has no menu of its own.
func (a *app) toggleAgent() {
	if a.agentKind() == agentGrok {
		a.chooseAgent(agentCodex)
		return
	}
	a.chooseAgent(agentGrok)
}
