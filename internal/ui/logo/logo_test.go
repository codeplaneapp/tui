package logo

import (
	"testing"

	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestRender_Wide(t *testing.T) {
	t.Parallel()

	sty := styles.DefaultStyles()
	out := Render(&sty, "v0.0.0-test", false, Opts{
		FieldColor:   sty.LogoFieldColor,
		TitleColorA:  sty.LogoTitleColorA,
		TitleColorB:  sty.LogoTitleColorB,
		CharmColor:   sty.LogoCharmColor,
		VersionColor: sty.LogoVersionColor,
		Width:        140,
	})

	plain := ansi.Strip(out)
	require.Contains(t, plain, "CODEPLANE")
	require.NotContains(t, plain, "SMITHERS")
	require.NotContains(t, plain, "Charm")
}

func TestRender_Compact(t *testing.T) {
	t.Parallel()

	sty := styles.DefaultStyles()
	out := Render(&sty, "v0.0.0-test", true, Opts{
		FieldColor:   sty.LogoFieldColor,
		TitleColorA:  sty.LogoTitleColorA,
		TitleColorB:  sty.LogoTitleColorB,
		CharmColor:   sty.LogoCharmColor,
		VersionColor: sty.LogoVersionColor,
		Width:        80,
	})

	plain := ansi.Strip(out)
	require.Contains(t, plain, "CODEPLANE")
	require.NotContains(t, plain, "SMITHERS")
	require.NotContains(t, plain, "Charm")
}

func TestSmallRender(t *testing.T) {
	t.Parallel()

	sty := styles.DefaultStyles()
	out := SmallRender(&sty, 80)
	plain := ansi.Strip(out)

	require.Contains(t, plain, "CODEPLANE")
	require.NotContains(t, plain, "SMITHERS")
	require.NotContains(t, plain, "Charm")
}
