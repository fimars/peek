package app

import "github.com/egoist/mygo/ui"

// agentButton chooses the agent Ship and Review use, its icon alone as a
// menu button.
func (a *app) agentButton(c *ui.Context) {
	def := agentDefOf(a.agentKind())
	t := c.Theme()
	label := "Ship and Review with " + def.name
	tooltip := label
	switch {
	case a.push.running:
		tooltip = "Shipping with " + def.name
	case a.review.running:
		tooltip = "Reviewing with " + def.name
	}
	b := ui.Button(c.Key("agent"), "").Shrink(0).Label(label).Tooltip(tooltip).
		Disabled(a.push.running || a.review.running)
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
