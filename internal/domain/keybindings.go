package domain

import "fmt"

// Keybinding action constants.
const (
	ActionRate0 = "rate_0"
	ActionRate1 = "rate_1"
	ActionRate2 = "rate_2"
	ActionRate3 = "rate_3"
	ActionRate4 = "rate_4"
	ActionRate5 = "rate_5"

	ActionFlagPick   = "flag_pick"
	ActionFlagReject = "flag_reject"
	ActionFlagClear  = "flag_clear"

	ActionLabelRed    = "label_red"
	ActionLabelOrange = "label_orange"
	ActionLabelYellow = "label_yellow"
	ActionLabelGreen  = "label_green"
	ActionLabelBlue   = "label_blue"
	ActionLabelPurple = "label_purple"
	ActionLabelClear  = "label_clear"

	ActionNavNext    = "nav_next"
	ActionNavPrev    = "nav_prev"
	ActionNavNextRow = "nav_next_row"
	ActionNavPrevRow = "nav_prev_row"

	ActionToggleFullscreen = "toggle_fullscreen"
	ActionToggleDetail     = "toggle_detail"
	ActionZoomIn           = "zoom_in"
	ActionZoomOut          = "zoom_out"

	ActionOpenInApp       = "open_in_app"
	ActionAddToCollection = "add_to_collection"
	ActionSelectAll       = "select_all"
	ActionDeselectAll     = "deselect_all"

	ActionUndo   = "undo"
	ActionRedo   = "redo"
	ActionDelete = "delete"
)

type ErrKeybindingConflict struct {
	Combo          string
	ConflictAction string
}

func (e *ErrKeybindingConflict) Error() string {
	return fmt.Sprintf("keybinding %s conflicts with action %s", e.Combo, e.ConflictAction)
}
