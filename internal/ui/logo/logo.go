// Package logo renders a Crush wordmark in a stylized way.
package logo

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// Opts are the options for rendering the title art.
type Opts struct {
	FieldColor   color.Color // diagonal lines
	TitleColorA  color.Color // left gradient ramp point
	TitleColorB  color.Color // right gradient ramp point
	CharmColor   color.Color // brand text color
	VersionColor color.Color // Version text color
	Width        int         // width of the rendered logo, used for truncation
}

// Render renders the Crush logo. Set the argument to true to render the
// narrow version, intended for use in a sidebar.
//
// The compact argument determines whether it renders compact for the sidebar
// or wider for the main pane.
func Render(s *styles.Styles, version string, compact bool, o Opts) string {
	title := styles.ApplyBoldForegroundGrad(s, "CRUSH", o.TitleColorA, o.TitleColorB)

	if version != "" {
		versionText := lipgloss.NewStyle().Foreground(o.VersionColor).Render(" " + version)
		title += versionText
	}

	if o.Width > 0 {
		// Truncate the logo to the specified width.
		title = ansi.Truncate(title, o.Width, "")
	}
	return title
}

// SmallRender renders a smaller version of the Crush logo, suitable for
// smaller windows or sidebar usage.
func SmallRender(t *styles.Styles, width int) string {
	title := styles.ApplyBoldForegroundGrad(t, "CRUSH", t.Secondary, t.Primary)
	return title
}
