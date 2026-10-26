package app

import (
	"peek/internal/git"

	"github.com/egoist/mygo/ui"
)

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// markIcon is a brand mark, filled with the text color, in its own viewBox.
func markIcon(viewBox, paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + viewBox + `" fill="currentColor" fill-rule="evenodd">` + paths + `</svg>`))
}

var (
	historyIcon  = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M10 2.5v11"/>`)
	sidebarIcon  = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M6 2.5v11"/>`)
	folderIcon   = icon(`<path d="M1.5 4.5a1 1 0 0 1 1-1h3.6l1.4 1.5h5.9a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1h-10.9a1 1 0 0 1-1-1z"/>`)
	worktreeIcon = icon(`<path d="M10.5 2.5l3 3-7.5 7.5H3v-3z"/><path d="M8.5 4.5l3 3"/>`)
	chevronIcon  = icon(`<path d="M4 6l4 4 4-4" stroke-width="1.8"/>`)
	branchIcon   = icon(`<circle cx="4.5" cy="3.5" r="1.5"/><circle cx="4.5" cy="12.5" r="1.5"/><circle cx="11.5" cy="5" r="1.5"/><path d="M4.5 5v6M11.5 6.5c0 2.5-3 2.5-5.5 4.6"/>`)
	pullIcon     = icon(`<circle cx="4.5" cy="3.5" r="1.5"/><circle cx="4.5" cy="12.5" r="1.5"/><circle cx="11.5" cy="12.5" r="1.5"/><path d="M4.5 5v6M11.5 11V6.5a2 2 0 0 0-2-2H7.5"/><path d="M9 3l-1.5 1.5L9 6"/>`)
	checkIcon    = icon(`<path d="M3.5 8.5l3 3 6-7" stroke-width="1.6"/>`)
	pullArrow    = icon(`<path d="M8 2.5v10M4 8.5l4 4 4-4" stroke-width="1.6"/>`)
	stopIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"><rect x="3.5" y="3.5" width="9" height="9" rx="1.5"/></svg>`))
	repoIcon     = icon(`<path d="M3 2.5h8.5a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4.5A1.5 1.5 0 0 1 3 12V2.5z"/><path d="M3 11.5a1.5 1.5 0 0 1 1.5-1.5h8"/><path d="M6 13.5v2l1-.7 1 .7v-2"/>`)
	previewIcon  = icon(`<path d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8 12.1 12.5 8 12.5 1.5 8 1.5 8z"/><circle cx="8" cy="8" r="2"/>`)
	splitIcon    = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M8 2.5v11M3.5 6h2.5M10 6h2.5M3.5 9h2.5M10 9h2.5"/>`)
	unifiedIcon  = icon(`<rect x="1.5" y="2.5" width="13" height="11" rx="2"/><path d="M4 5.5h8M4 8h8M4 10.5h5"/>`)
	// shortcutsIcon is the shortcuts' to a request, a bolt, as quick
	// actions'; sendIcon sends it.
	shortcutsIcon   = icon(`<path d="M9 1.8L3.6 9h4.1l-.9 5.2L12.4 7H8.2z" fill="currentColor" stroke-width="1"/>`)
	copyIcon        = icon(`<rect x="5.5" y="5.5" width="8" height="8" rx="1.5"/><path d="M10.5 5.5V3.5a1 1 0 0 0-1-1h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2"/>`)
	checkCircleIcon = icon(`<circle cx="8" cy="8" r="6.5"/><path d="M5.2 8.2l2 2 3.8-4.2" stroke-width="1.6"/>`)
	warningIcon     = icon(`<path d="M8 2.2L14.3 13a.6.6 0 0 1-.5.9H2.2a.6.6 0 0 1-.5-.9z"/><path d="M8 6.5v3" stroke-width="1.6"/><circle cx="8" cy="11.6" r=".4" fill="currentColor"/>`)
	sendIcon        = icon(`<path d="M8 13.5v-11M3.5 7L8 2.5 12.5 7" stroke-width="1.8"/>`)
	commentIcon     = icon(`<path d="M2.5 3.5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1v6.5a1 1 0 0 1-1 1H7l-3 2.5v-2.5h-.5a1 1 0 0 1-1-1z"/>`)
	// Codex's and Grok Build's marks, from thesvg.org's icon set (MIT).
	codexIcon = markIcon("0 0 24 24", `<path clip-rule="evenodd" d="M8.086.457a6.105 6.105 0 013.046-.415c1.333.153 2.521.72 3.564 1.7a.117.117 0 00.107.029c1.408-.346 2.762-.224 4.061.366l.063.03.154.076c1.357.703 2.33 1.77 2.918 3.198.278.679.418 1.388.421 2.126a5.655 5.655 0 01-.18 1.631.167.167 0 00.04.155 5.982 5.982 0 011.578 2.891c.385 1.901-.01 3.615-1.183 5.14l-.182.22a6.063 6.063 0 01-2.934 1.851.162.162 0 00-.108.102c-.255.736-.511 1.364-.987 1.992-1.199 1.582-2.962 2.462-4.948 2.451-1.583-.008-2.986-.587-4.21-1.736a.145.145 0 00-.14-.032c-.518.167-1.04.191-1.604.185a5.924 5.924 0 01-2.595-.622 6.058 6.058 0 01-2.146-1.781c-.203-.269-.404-.522-.551-.821a7.74 7.74 0 01-.495-1.283 6.11 6.11 0 01-.017-3.064.166.166 0 00.008-.074.115.115 0 00-.037-.064 5.958 5.958 0 01-1.38-2.202 5.196 5.196 0 01-.333-1.589 6.915 6.915 0 01.188-2.132c.45-1.484 1.309-2.648 2.577-3.493.282-.188.55-.334.802-.438.286-.12.573-.22.861-.304a.129.129 0 00.087-.087A6.016 6.016 0 015.635 2.31C6.315 1.464 7.132.846 8.086.457zm-.804 7.85a.848.848 0 00-1.473.842l1.694 2.965-1.688 2.848a.849.849 0 001.46.864l1.94-3.272a.849.849 0 00.007-.854l-1.94-3.393zm5.446 6.24a.849.849 0 000 1.695h4.848a.849.849 0 000-1.696h-4.848z"/>`)
	grokIcon  = markIcon("0 0 1024 1024", `<path d="M395.479 633.828L735.91 381.105C752.599 368.715 776.454 373.548 784.406 392.792C826.26 494.285 807.561 616.253 724.288 699.996C641.016 783.739 525.151 802.104 419.247 760.277L303.556 814.143C469.49 928.202 670.987 899.995 796.901 773.282C896.776 672.843 927.708 535.937 898.785 412.476L899.047 412.739C857.105 231.37 909.358 158.874 1016.4 10.6326C1018.93 7.11771 1021.47 3.60279 1024 0L883.144 141.651V141.212L395.392 633.916"/><path d="M325.226 695.251C206.128 580.84 226.662 403.776 328.285 301.668C403.431 226.097 526.549 195.254 634.026 240.596L749.454 186.994C728.657 171.88 702.007 155.623 671.424 144.2C533.19 86.9942 367.693 115.465 255.323 228.382C147.234 337.081 113.244 504.215 171.613 646.833C215.216 753.423 143.739 828.818 71.7385 904.916C46.2237 931.893 20.6216 958.87 0 987.429L325.139 695.339"/>`)
)

// githubIcon is GitHub's mark, from Primer's Octicons (MIT), filled.
var githubIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z"/></svg>`))

// gitlabIcon is GitLab's tanuki, in its classic oranges, from GitLab's mark (MIT).
var gitlabIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 25 24"><path d="m24.507 9.5-.034-.09L21.082.562a.896.896 0 0 0-1.694.091l-2.29 7.01H7.825L5.535.653a.898.898 0 0 0-1.694-.09L.451 9.411.416 9.5a6.297 6.297 0 0 0 2.09 7.278l.012.01.03.022 5.16 3.867 2.56 1.935 1.554 1.176a1.051 1.051 0 0 0 1.268 0l1.555-1.176 2.56-1.935 5.197-3.89.014-.01A6.297 6.297 0 0 0 24.507 9.5Z" fill="#E24329"/><path d="m24.507 9.5-.034-.09a11.44 11.44 0 0 0-4.56 2.051l-7.447 5.632 4.742 3.584 5.197-3.89.014-.01A6.297 6.297 0 0 0 24.507 9.5Z" fill="#FC6D26"/><path d="m7.707 20.677 2.56 1.935 1.555 1.176a1.051 1.051 0 0 0 1.268 0l1.555-1.176 2.56-1.935-4.743-3.584-4.755 3.584Z" fill="#FCA326"/><path d="M5.01 11.461a11.43 11.43 0 0 0-4.56-2.05L.416 9.5a6.297 6.297 0 0 0 2.09 7.278l.012.01.03.022 5.16 3.867 4.745-3.584-7.444-5.632Z" fill="#FC6D26"/></svg>`))

// hostIcon is GitLab's mark for a GitLab host, and GitHub's for any other.
func hostIcon(kind git.Kind) *ui.SVG {
	if kind == git.KindGitLab {
		return gitlabIcon
	}
	return githubIcon
}
