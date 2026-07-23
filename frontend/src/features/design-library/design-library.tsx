// The in-app design library (#/design-library): specimens and product share one
// implementation (frontend/CLAUDE.md §6). Everything rendered here is driven by
// the compiler's outputs — tokens-reference.json for the inventory, the live
// CSS variables for values — never a hand-listed parallel of either (C15).
//
// ponytail: copy is literal by sanction (task-24 spec) — this is a dev-facing
// surface; C14 keys arrive when product chrome adopts these strings.

import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { Badge, type BadgeHue, type BadgeSize, type BadgeStyle } from "@/components/badge/badge";
import { Button, type ButtonRung, type ButtonSize } from "@/components/button/button";
import { Checkbox } from "@/components/checkbox/checkbox";
import { ControlGroup } from "@/components/control-group/control-group";
import { ControlRow } from "@/components/control-row/control-row";
import { Icon, type IconConcept } from "@/components/icon/icon";
import { Kbd, type KbdSize, type KbdStyle, KbdGroup } from "@/components/kbd/kbd";
import {
    Menu,
    MenuItem,
    MenuSection,
    MenuSectionHeader,
    MenuSeparator,
    MenuTrigger,
    SubmenuTrigger,
} from "@/components/menu/menu";
import { PanelSection } from "@/components/panel-section/panel-section";
import { Rating } from "@/components/rating/rating";
import { Row } from "@/components/row/row";
import {
    Segment,
    SegmentedControl,
    type SegmentedControlSize,
} from "@/components/segmented-control/segmented-control";
import { Select, SelectItem } from "@/components/select/select";
import { Switch } from "@/components/switch/switch";
import { TextField } from "@/components/text-field/text-field";
import { ToggleButton } from "@/components/toggle-button/toggle-button";
import { Tree, type TreeNodeData } from "@/components/tree/tree";
import { getTheme, setTheme, themes, type Theme } from "@/lib/theme";
import reference from "@/styles/tokens-reference.json";
import styles from "./design-library.module.css";

const RUNGS = ["ghost", "outline", "tint", "fill", "hero"] as const satisfies readonly ButtonRung[];

const FORCED_STATES = ["rest", "hovered", "pressed", "focus-visible"] as const;
/** "focused" exists for the text-input family, whose ring shows on ANY focus. */
type ForcedState = (typeof FORCED_STATES)[number] | "focused";

/** The control-height ladder (§8/D33), shared by every sized chrome primitive. */
const CONTROL_SIZES = [
    { key: "xs", label: "xs · 16" },
    { key: "sm", label: "sm · 20" },
    { key: "md", label: "md · 24" },
    { key: "lg", label: "lg · 28" },
] as const satisfies readonly { key: ButtonSize; label: string }[];

/** One column of a size×state grid: a size row is crossed with these. Flags are a
 * superset across primitives; each cell closure reads the ones it needs. */
interface StateCol {
    key: string;
    label: string;
    forced?: ForcedState;
    selected?: boolean;
    checked?: boolean;
    mixed?: boolean;
    on?: boolean;
    invalid?: boolean;
    disabled?: boolean;
}

/** Freezes one RAC interaction state onto the specimen inside, so every matrix
 * cell exercises the product's own CSS. RAC computes data-attributes from its
 * interaction hooks and drops same-named props, so the attribute is stamped
 * IMPERATIVELY after mount — React leaves attributes it never rendered alone,
 * and RAC only touches them on real interaction, which specimens never receive.
 * This is the library's standing specimen device for primitive matrices. */
function ForcedStateCell({ state, children }: { state: ForcedState; children: ReactNode }) {
    const cellReference = useRef<HTMLSpanElement>(null);
    useEffect(() => {
        if (state === "rest") return;
        // The stateful element differs per family: the text-input family keys
        // hover/focus on the INPUT; the press family keys on its root. The
        // checkbox family's hidden inputs are type=checkbox — never targets.
        const root = cellReference.current;
        const target =
            root?.querySelector("input:not([type=checkbox])") ??
            root?.querySelector("button, label");
        target?.setAttribute(`data-${state}`, "true");
    }, [state]);
    return (
        <span ref={cellReference} className={styles.matrixCell}>
            {children}
        </span>
    );
}

/** A cell that carries no forced state (disabled, static). Mirrors ForcedStateCell's
 * wrapper so both flow as grid items. */
function StaticCell({ children }: { children: ReactNode }) {
    return <span className={styles.matrixCell}>{children}</span>;
}

/** Generic size×state grid: rows (usually the size ladder) × cols (states), one
 * specimen per intersection. The cell closure owns rendering — it decides whether
 * a column is a forced interaction state or a prop-driven one. */
function SizeStateGrid<R extends { key: string; label: string }, C extends { key: string; label: string }>({
    rows,
    cols,
    cell,
}: {
    rows: readonly R[];
    cols: readonly C[];
    cell: (row: R, col: C) => ReactNode;
}) {
    return (
        <div
            className={styles.grid2d}
            style={{ gridTemplateColumns: `72px repeat(${cols.length}, max-content)` }}
        >
            <span />
            {cols.map((col) => (
                <span key={col.key} className={styles.matrixLabel}>{col.label}</span>
            ))}
            {rows.map((row) => (
                <Fragment key={row.key}>
                    <span className={styles.matrixLabel}>{row.label}</span>
                    {cols.map((col) => (
                        <Fragment key={col.key}>{cell(row, col)}</Fragment>
                    ))}
                </Fragment>
            ))}
        </div>
    );
}

interface ReferenceToken {
    path: string;
    type: string;
    varying: boolean;
    role?: string;
    pin?: string;
    /** Invariant tokens: one CSS string. Varying tokens: per-theme map. */
    css?: string | Record<string, string>;
    /** Typography composites carry per-property variables instead of css. */
    variables?: Record<string, string>;
}

const tokens = reference.tokens as ReferenceToken[];

const byPrefix = (prefix: string): ReferenceToken[] =>
    tokens.filter((token) => token.path.startsWith(prefix));

/** The invariant CSS string for a token path (size/space rungs are theme-invariant). */
const invariantCss = (path: string): string => String(tokens.find((token) => token.path === path)?.css ?? "");

/** color.<hue>.<step> paths → the hue list, in SPECTRAL order (the reference file
 * is alphabetical by path; a palette reads by hue angle, gray last). */
const hueAngle = (hue: string): number => {
    const solid = tokens.find((token) => token.path === `color.${hue}.solid`);
    const match = /oklch\([\d.]+ ([\d.]+) ([\d.]+)/.exec(String(solid?.css ?? ""));
    return match === null || match[1] === "0" ? Number.POSITIVE_INFINITY : Number(match[2]);
};
const hueNames = [...new Set(
    byPrefix("color.").map((token) => token.path.split(".")[1]),
)].sort((first, second) => hueAngle(first) - hueAngle(second));

const HUE_STEPS = ["solid", "tint", "line", "ring"] as const;

function ThemeSwitcher() {
    const [activeTheme, setActiveTheme] = useState<Theme>(() => getTheme());
    return (
        <SegmentedControl
            aria-label="Theme"
            value={activeTheme}
            onChange={(key) => {
                const next = key as Theme;
                setTheme(next);
                setActiveTheme(next);
            }}
        >
            {themes.map((theme) => (
                <Segment key={theme} id={theme}>{theme}</Segment>
            ))}
        </SegmentedControl>
    );
}

// ── The sizing system (§8/D33) ────────────────────────────────────────────────

interface SizingRung {
    token: string;
    name: string;
    size: ButtonSize;
    text: string;
    icon: string;
    pad: string;
    target: string;
    use: string;
}

const SIZING_RUNGS: readonly SizingRung[] = [
    {
        token: "size.control-xs",
        name: "control-xs",
        size: "xs",
        text: "control-xs · 10",
        icon: "12",
        pad: "space.1 · 4",
        target: "16 · mouse-only (§28)",
        use: "inspector inline-edit — matches the read-only row (zero-shift)",
    },
    {
        token: "size.control-sm",
        name: "control-sm",
        size: "sm",
        text: "control-sm · 11",
        icon: "14",
        pad: "space.2 · 8",
        target: "24 (fills its row)",
        use: "dense chips, secondary icon-buttons",
    },
    {
        token: "size.control-md",
        name: "control-md",
        size: "md",
        text: "control · 12",
        icon: "16",
        pad: "space.3 · 12",
        target: "24",
        use: "the default — buttons, fields, toolbar chrome",
    },
    {
        token: "size.control-lg",
        name: "control-lg",
        size: "lg",
        text: "control-lg · 13",
        icon: "18",
        pad: "space.4 · 16",
        target: "28",
        use: "prominent — dialog CTAs, hero spots",
    },
];

function SizingSystem({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Sizing system — the proportional ladder (§8/D33)</h2>
            <p className={styles.note}>
                <code className={styles.formula}>16 / 20 / 24 / 28</code>
                <span>Each tier is a full bundle — text, icon, indicator, pad, height — that scales into the
                    tier height. Text + icon values are first-pass, eye-tune on render.</span>
            </p>
            <div className={styles.sizeTableWrap}>
                <table className={styles.sizeTable}>
                    <thead>
                        <tr className="alx-type-label-sm">
                            <th>token</th>
                            <th>height</th>
                            <th>text</th>
                            <th>icon</th>
                            <th>inline pad</th>
                            <th>hit target</th>
                            <th>use for</th>
                        </tr>
                    </thead>
                    <tbody>
                        {SIZING_RUNGS.map((rung) => (
                            <tr key={rung.token}>
                                <td className="alx-type-data-sm">{rung.name}</td>
                                <td className="alx-type-data-sm">{invariantCss(rung.token)}</td>
                                <td className="alx-type-data-sm">{rung.text}</td>
                                <td className="alx-type-data-sm">{rung.icon}</td>
                                <td className="alx-type-data-sm">{rung.pad}</td>
                                <td className="alx-type-data-sm">{rung.target}</td>
                                <td className="alx-type-caption">{rung.use}</td>
                            </tr>
                        ))}
                        <tr className={styles.floorRow}>
                            <td className="alx-type-data-sm">row-text</td>
                            <td className="alx-type-data-sm">{invariantCss("size.row-text")}</td>
                            <td className="alx-type-data-sm">data-sm · 11</td>
                            <td className="alx-type-data-sm">—</td>
                            <td className="alx-type-data-sm">—</td>
                            <td className="alx-type-data-sm">— (read-only)</td>
                            <td className="alx-type-caption">the display floor — EXIF values (ISO, ƒ, shutter); not a control</td>
                        </tr>
                    </tbody>
                </table>
            </div>
            <p className={styles.note}>
                One rung, three primitives — text, icon, and control all scale together at each height:
            </p>
            <div className={styles.sizeStrip}>
                {SIZING_RUNGS.map((rung) => (
                    <div key={rung.token} className={styles.sizeStripRow}>
                        <span className={styles.matrixLabel}>{rung.name} · {invariantCss(rung.token)}</span>
                        <Button size={rung.size}>Import</Button>
                        <ToggleButton size={rung.size} defaultSelected>Raw</ToggleButton>
                        <TextField label="ISO" size={rung.size} defaultValue="400" />
                    </div>
                ))}
            </div>
            <p className={styles.note}>
                Checkbox / Switch / Rating scale their indicator too — the box, track, and star ride the icon
                ramp (12/14/16/18) via the tier's <code className={styles.formula}>--alx-size-icon</code>
                reassignment. control-xs (16) and the row-text floor (16) share a height: one is the interactive
                inline-edit tier (mouse-only), the other the read-only display floor.
            </p>
        </section>
    );
}

// ── Primitives ────────────────────────────────────────────────────────────────

function ButtonMatrix({ id }: SectionProps) {
    const [pressCount, setPressCount] = useState(0);
    const sizeCols: readonly StateCol[] = [
        { key: "rest", label: "rest", forced: "rest" },
        { key: "hovered", label: "hovered", forced: "hovered" },
        { key: "pressed", label: "pressed", forced: "pressed" },
        { key: "focus", label: "focus-visible", forced: "focus-visible" },
        { key: "disabled", label: "disabled", disabled: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Button — rungs × states, and the size ladder × states</h2>
            <div className={styles.matrix}>
                <div className={styles.matrixRow}>
                    <span className={styles.matrixLabel} />
                    {FORCED_STATES.map((state) => (
                        <span key={state} className={styles.matrixLabel}>{state}</span>
                    ))}
                    <span className={styles.matrixLabel}>disabled</span>
                    <span className={styles.matrixLabel}>live</span>
                </div>
                {RUNGS.map((rung) => (
                    <div key={rung} className={styles.matrixRow}>
                        <span className={styles.matrixLabel}>{rung}</span>
                        {FORCED_STATES.map((state) => (
                            <ForcedStateCell key={state} state={state}>
                                <Button rung={rung}>Import</Button>
                            </ForcedStateCell>
                        ))}
                        <span className={styles.matrixCell}>
                            <Button rung={rung} isDisabled>Import</Button>
                        </span>
                        <span className={styles.matrixCell}>
                            <Button rung={rung} onPress={() => setPressCount((count) => count + 1)}>
                                Import
                            </Button>
                        </span>
                    </div>
                ))}
            </div>
            <p className={styles.subHead}>size × states (outline rung)</p>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={sizeCols}
                cell={(size, col) => {
                    const button = (
                        <Button rung="outline" size={size.key} isDisabled={col.disabled}>Import</Button>
                    );
                    return col.forced
                        ? <ForcedStateCell state={col.forced}>{button}</ForcedStateCell>
                        : <StaticCell>{button}</StaticCell>;
                }}
            />
            <p className={styles.note}>
                <span className={styles.pressReadout}>{pressCount > 0 ? `${pressCount} presses` : ""}</span>
            </p>
        </section>
    );
}

function ToggleButtonMatrix({ id }: SectionProps) {
    const cols: readonly StateCol[] = [
        { key: "rest", label: "rest", forced: "rest" },
        { key: "hovered", label: "hovered", forced: "hovered" },
        { key: "selected", label: "selected", forced: "rest", selected: true },
        { key: "selhover", label: "sel+hover", forced: "hovered", selected: true },
        { key: "disabled", label: "disabled", disabled: true },
        { key: "dison", label: "disabled on", disabled: true, selected: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>ToggleButton — the boolean register × size (§14: on = fill)</h2>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={cols}
                cell={(size, col) => {
                    const toggle = (
                        <ToggleButton size={size.key} defaultSelected={col.selected} isDisabled={col.disabled}>
                            Raw
                        </ToggleButton>
                    );
                    return col.forced
                        ? <ForcedStateCell state={col.forced}>{toggle}</ForcedStateCell>
                        : <StaticCell>{toggle}</StaticCell>;
                }}
            />
        </section>
    );
}

const SEGMENTED_SIZES = [
    { key: "sm", label: "sm · 20" },
    { key: "md", label: "md · 24" },
    { key: "lg", label: "lg · 28" },
] as const satisfies readonly { key: SegmentedControlSize; label: string }[];

function SegmentedControlSpecimens({ id }: SectionProps) {
    const cols: readonly StateCol[] = [
        { key: "rest", label: "rest" },
        { key: "disabled", label: "disabled", disabled: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>SegmentedControl — single-select track × size</h2>
            <SizeStateGrid
                rows={SEGMENTED_SIZES}
                cols={cols}
                cell={(size, col) => (
                    <StaticCell>
                        <SegmentedControl
                            aria-label={`View ${size.key} ${col.key}`}
                            size={size.key}
                            defaultValue="loupe"
                            isDisabled={col.disabled}
                        >
                            <Segment id="grid">Grid</Segment>
                            <Segment id="loupe">Loupe</Segment>
                            <Segment id="compare">Compare</Segment>
                        </SegmentedControl>
                    </StaticCell>
                )}
            />
            <p className={styles.subHead}>content — text · icon · icon+text (md)</p>
            <div className={styles.swatchRow}>
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>text</span>
                    <SegmentedControl aria-label="View mode" defaultValue="grid">
                        <Segment id="grid">Grid</Segment>
                        <Segment id="loupe">Loupe</Segment>
                        <Segment id="compare">Compare</Segment>
                    </SegmentedControl>
                </span>
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>icon</span>
                    <SegmentedControl aria-label="Flag" defaultValue="none">
                        <Segment id="reject" aria-label="Reject"><Icon concept="reject" /></Segment>
                        <Segment id="none" aria-label="No flag"><Icon concept="mixed" /></Segment>
                        <Segment id="flag" aria-label="Flag"><Icon concept="flag" /></Segment>
                    </SegmentedControl>
                </span>
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>icon + text</span>
                    <SegmentedControl aria-label="Flag" defaultValue="flag">
                        <Segment id="reject"><Icon concept="reject" />Reject</Segment>
                        <Segment id="flag"><Icon concept="flag" />Flag</Segment>
                    </SegmentedControl>
                </span>
            </div>
        </section>
    );
}

function CheckboxMatrix({ id }: SectionProps) {
    const cols: readonly StateCol[] = [
        { key: "rest", label: "rest", forced: "rest" },
        { key: "hovered", label: "hovered", forced: "hovered" },
        { key: "checked", label: "checked", forced: "rest", checked: true },
        { key: "chkhover", label: "chk+hover", forced: "hovered", checked: true },
        { key: "mixed", label: "mixed", forced: "rest", mixed: true },
        { key: "invalid", label: "invalid", forced: "rest", invalid: true },
        { key: "disabled", label: "disabled", disabled: true },
        { key: "discheck", label: "dis+checked", disabled: true, checked: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Checkbox — the toggles-on ledger row × size (§5)</h2>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={cols}
                cell={(size, col) => {
                    const checkbox = (
                        <Checkbox
                            size={size.key}
                            defaultSelected={col.checked}
                            isIndeterminate={col.mixed}
                            isInvalid={col.invalid}
                            isDisabled={col.disabled}
                        >
                            Reject
                        </Checkbox>
                    );
                    return col.forced
                        ? <ForcedStateCell state={col.forced}>{checkbox}</ForcedStateCell>
                        : <StaticCell>{checkbox}</StaticCell>;
                }}
            />
            <p className={styles.note}>The box rides the icon ramp (12/14/16/18); label, box, and hit-row scale together.</p>
        </section>
    );
}

function SwitchMatrix({ id }: SectionProps) {
    const cols: readonly StateCol[] = [
        { key: "rest", label: "rest", forced: "rest" },
        { key: "hovered", label: "hovered", forced: "hovered" },
        { key: "on", label: "on", forced: "rest", on: true },
        { key: "onhover", label: "on+hover", forced: "hovered", on: true },
        { key: "disabled", label: "disabled", disabled: true },
        { key: "dison", label: "disabled on", disabled: true, on: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Switch — the immediate-effect boolean × size (§5)</h2>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={cols}
                cell={(size, col) => {
                    const toggle = (
                        <Switch size={size.key} defaultSelected={col.on} isDisabled={col.disabled}>
                            Watch
                        </Switch>
                    );
                    return col.forced
                        ? <ForcedStateCell state={col.forced}>{toggle}</ForcedStateCell>
                        : <StaticCell>{toggle}</StaticCell>;
                }}
            />
            <p className={styles.note}>The track derives from the icon ramp (height = icon, width = 2·icon−4); it scales with the tier.</p>
        </section>
    );
}

function TextFieldMatrix({ id }: SectionProps) {
    const cols: readonly StateCol[] = [
        { key: "rest", label: "rest", forced: "rest" },
        { key: "focused", label: "focused", forced: "focused" },
        { key: "invalid", label: "invalid", forced: "rest", invalid: true },
        { key: "disabled", label: "disabled", disabled: true },
    ];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>TextField — the field composite × size (§25)</h2>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={cols}
                cell={(size, col) => {
                    const field = (
                        <TextField
                            label="ISO"
                            size={size.key}
                            defaultValue="400"
                            errorMessage="Out of range"
                            isInvalid={col.invalid}
                            isDisabled={col.disabled}
                        />
                    );
                    return col.forced
                        ? <ForcedStateCell state={col.forced}>{field}</ForcedStateCell>
                        : <StaticCell>{field}</StaticCell>;
                }}
            />
            <p className={styles.note}>
                The <code className={styles.formula}>sm</code> field is the inline-edit control — dropped in a
                row-list (24) row, its click target fills the row for the §8 24px hit-target floor.
            </p>
        </section>
    );
}

function RatingMatrix({ id }: SectionProps) {
    const [liveValue, setLiveValue] = useState<number | null>(3);
    const valueCols = [
        { key: "null", label: "null", value: null },
        { key: "1", label: "1", value: 1 },
        { key: "3", label: "3", value: 3 },
        { key: "5", label: "5", value: 5 },
    ] as const satisfies readonly { key: string; label: string; value: number | null }[];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Rating — the five-position readout × size (§14 fill = on)</h2>
            <SizeStateGrid
                rows={CONTROL_SIZES}
                cols={valueCols}
                cell={(size, col) => (
                    <StaticCell>
                        <Rating value={col.value} size={size.key} />
                    </StaticCell>
                )}
            />
            <p className={styles.note}>
                Stars ride the icon ramp (12/14/16/18); the tier scales them + the hit-row. Live — click the current value to clear:
                <Rating value={liveValue} onChange={setLiveValue} />
            </p>
        </section>
    );
}

const BADGE_STYLES = ["tint", "outline", "fill", "dot"] as const satisfies readonly BadgeStyle[];
const BADGE_HUES = [
    "red",
    "peach",
    "orange",
    "amber",
    "lime",
    "green",
    "teal",
    "cyan",
    "blue",
    "indigo",
    "purple",
    "magenta",
    "gray",
] as const satisfies readonly BadgeHue[];
const BADGE_SIZES = [
    { key: "inline", label: "inline · micro" },
    { key: "standard", label: "standard · label-sm" },
    { key: "prominent", label: "prominent · label" },
] as const satisfies readonly { key: BadgeSize; label: string }[];

function BadgeMatrix({ id }: SectionProps) {
    const styleCols = BADGE_STYLES.map((style) => ({ key: style, label: style }));
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Badge — the tagRecipes chip: size × style, then style × hue (§5)</h2>
            <SizeStateGrid
                rows={BADGE_SIZES}
                cols={styleCols}
                cell={(size, col) => (
                    <StaticCell>
                        <Badge hue="cyan" style={col.key} size={size.key}>{col.key}</Badge>
                    </StaticCell>
                )}
            />
            <p className={styles.subHead}>style × hue (standard)</p>
            {BADGE_STYLES.map((style) => (
                <div key={style} className={styles.matrix}>
                    <span className={styles.matrixLabel}>{style}</span>
                    <div className={styles.badgeRow}>
                        {BADGE_HUES.map((hue) => (
                            <Badge key={hue} hue={hue} style={style}>
                                {hue}
                            </Badge>
                        ))}
                    </div>
                </div>
            ))}
            <p className={styles.note}>
                {/* One span = one flex item; inside it the badge flows INLINE —
                    that inline flow is what the line-fit proof measures. */}
                <span>
                    Line fit: filed under{" "}
                    <Badge hue="cyan" size="inline">
                        RAW
                    </Badge>{" "}
                    pending review — the inline rung must not grow the line box.
                </span>
            </p>
            <p className={styles.note}>
                Role bindings (tagRecipes.sizes): inline = micro (10px) · standard = label-sm (11px) · prominent =
                label (12px) — one point apart by design; the box, not the font, separates the rungs.
            </p>
        </section>
    );
}

const KBD_STYLES = ["flat", "keycap"] as const satisfies readonly KbdStyle[];
const KBD_MODIFIERS = ["command", "shift", "option", "control", "return", "delete"] as const satisfies readonly IconConcept[];
const KBD_LETTERS = ["P", "K", "Esc"] as const;
const KBD_SIZES = [
    { key: "xs", label: "xs · 16 (dense)" },
    { key: "sm", label: "sm · 20 (menu)" },
    { key: "md", label: "md · 24" },
] as const satisfies readonly { key: KbdSize; label: string }[];

function KbdMatrix({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Kbd — the keyboard-shortcut keycap: style × keys, and composed combos</h2>
            <p className={styles.subHead}>single caps — modifier icons + text keys · flat (tinted) vs keycap (bordered face + bottom rule)</p>
            {KBD_STYLES.map((style) => (
                <div key={style} className={styles.matrix}>
                    <span className={styles.matrixLabel}>{style}</span>
                    <div className={styles.badgeRow}>
                        {KBD_MODIFIERS.map((concept) => (
                            <Kbd key={concept} style={style} icon={concept} />
                        ))}
                        {KBD_LETTERS.map((key) => (
                            <Kbd key={key} style={style}>
                                {key}
                            </Kbd>
                        ))}
                    </div>
                </div>
            ))}
            <p className={styles.subHead}>composed combos (KbdGroup — one cap per key, the shadcn model)</p>
            {KBD_STYLES.map((style) => (
                <div key={style} className={styles.matrix}>
                    <span className={styles.matrixLabel}>{style}</span>
                    <div className={styles.badgeRow}>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style}>K</Kbd>
                        </KbdGroup>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style} icon="shift" />
                            <Kbd style={style}>P</Kbd>
                        </KbdGroup>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style} icon="delete" />
                        </KbdGroup>
                    </div>
                </div>
            ))}
            <p className={styles.subHead}>size ladder — caps ride the D33 control-size bundle (xs 16 / sm 20 / md 24); text + icon derive from the tier (text-box-trim centers the glyph + frees xs). Stops at md: mono ceilings at 12px</p>
            {KBD_SIZES.map(({ key, label }) => (
                <div key={key} className={styles.matrix}>
                    <span className={styles.matrixLabel}>{label}</span>
                    <div className={styles.badgeRow}>
                        <Kbd size={key} icon="command" />
                        <Kbd size={key}>K</Kbd>
                        <KbdGroup>
                            <Kbd size={key} icon="command" />
                            <Kbd size={key} icon="shift" />
                            <Kbd size={key}>P</Kbd>
                        </KbdGroup>
                        <Kbd size={key} style="keycap" icon="command" />
                        <KbdGroup>
                            <Kbd size={key} style="keycap" icon="command" />
                            <Kbd size={key} style="keycap">
                                K
                            </Kbd>
                        </KbdGroup>
                    </div>
                </div>
            ))}
            <p className={styles.note}>
                Neutral machinery — hue-free. flat honors §6 flat chrome (fill XOR border, D32); keycap leans on the §6
                keyboard-hint genre carve-out, lifting via a doubled bottom border (never a shadow). Modifier keys
                (⌘⇧⌥⌃↵⌫) render as vector icons — the Mac symbols mush as 11px font text, so they ride the icon
                registry instead. The Menu consumes flat: a bare-string shortcut renders one text cap, a KbdGroup
                renders composed caps.
            </p>
        </section>
    );
}

function SelectSpecimens({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Select — the dropdown trigger (control-container) + the §6 transient popover</h2>
            <p className={styles.subHead}>size ladder — the trigger rides the container material</p>
            <div className={styles.sizeStrip}>
                {CONTROL_SIZES.map((s) => (
                    <div key={s.key} className={styles.sizeStripRow}>
                        <span className={styles.matrixLabel}>{s.label}</span>
                        <Select label={`View ${s.key}`} size={s.key} defaultSelectedKey="loupe">
                            <SelectItem id="grid">Grid</SelectItem>
                            <SelectItem id="loupe">Loupe</SelectItem>
                            <SelectItem id="compare">Compare</SelectItem>
                        </Select>
                    </div>
                ))}
            </div>
            <p className={styles.subHead}>in a ControlRow — the reference "label + dropdown" row</p>
            <div className={styles.panelSpecimen}>
                <ControlGroup>
                    <ControlRow label="Type">
                        <Select label="Chart type" defaultSelectedKey="line">
                            <SelectItem id="line">Line</SelectItem>
                            <SelectItem id="bar">Bar</SelectItem>
                            <SelectItem id="area">Area</SelectItem>
                        </Select>
                    </ControlRow>
                    <ControlRow label="X-axis">
                        <Select label="X-axis" placeholder="Choose…">
                            <SelectItem id="date">Date</SelectItem>
                            <SelectItem id="quarter">Quarter</SelectItem>
                        </Select>
                    </ControlRow>
                </ControlGroup>
            </div>
            <p className={styles.note}>The overlay rides the shared §6 transient shell (components/popover) — click to open (surface.transient + occlusion shadow + transient radius).</p>
        </section>
    );
}

function MenuSpecimens({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Menu — the §6 transient&apos;s second tenant (role=menu on the shared Popover)</h2>
            <p className={styles.subHead}>click a trigger to open — action, single-select, mixed-icon alignment, and multi-select</p>
            <div className={styles.menuTriggers}>
                <MenuTrigger>
                    <Button rung="ghost">
                        Actions
                        <Icon concept="disclose" className={styles.triggerChevron} />
                    </Button>
                    <Menu aria-label="Asset actions">
                        <SubmenuTrigger>
                            <MenuItem id="rate" icon="rating">Rate</MenuItem>
                            <Menu aria-label="Rate">
                                <MenuItem id="r0">None</MenuItem>
                                <MenuItem id="r1">1 star</MenuItem>
                                <MenuItem id="r2">2 stars</MenuItem>
                                <MenuItem id="r3">3 stars</MenuItem>
                                <MenuItem id="r4">4 stars</MenuItem>
                                <MenuItem id="r5">5 stars</MenuItem>
                            </Menu>
                        </SubmenuTrigger>
                        <MenuItem id="pick" icon="flag" shortcut="P">Flag as pick</MenuItem>
                        <MenuItem id="reject" icon="reject" shortcut="X">Reject</MenuItem>
                        <MenuSeparator />
                        <MenuSection>
                            <MenuSectionHeader>More</MenuSectionHeader>
                            <MenuItem id="meta" icon="settings">Metadata settings…</MenuItem>
                            <MenuItem id="hide" description="Stays in the catalog; hidden from this view.">
                                Hide from grid
                            </MenuItem>
                            <MenuItem id="edit" isDisabled>Open in external editor</MenuItem>
                        </MenuSection>
                        <MenuSeparator />
                        <MenuItem
                            id="remove"
                            isDestructive
                            shortcut={
                                <KbdGroup>
                                    <Kbd icon="command" />
                                    <Kbd icon="delete" />
                                </KbdGroup>
                            }
                        >
                            Remove from catalog…
                        </MenuItem>
                    </Menu>
                </MenuTrigger>

                <MenuTrigger>
                    <Button rung="ghost">
                        View
                        <Icon concept="disclose" className={styles.triggerChevron} />
                    </Button>
                    <Menu aria-label="View mode" selectionMode="single" defaultSelectedKeys={["loupe"]}>
                        <MenuItem id="grid">Grid</MenuItem>
                        <MenuItem id="loupe">Loupe</MenuItem>
                        <MenuItem id="compare">Compare</MenuItem>
                        <MenuItem id="cull">Cull</MenuItem>
                    </Menu>
                </MenuTrigger>

                <MenuTrigger>
                    <Button rung="ghost">
                        Mixed icons
                        <Icon concept="disclose" className={styles.triggerChevron} />
                    </Button>
                    <Menu aria-label="Mixed icons">
                        <MenuItem id="space" icon="settings">Space settings</MenuItem>
                        <MenuItem id="fav" icon="flag">Add to favorites</MenuItem>
                        <MenuItem id="rename">Rename</MenuItem>
                        <MenuItem id="dupe">Duplicate</MenuItem>
                    </Menu>
                </MenuTrigger>

                <MenuTrigger>
                    <Button rung="ghost">
                        Columns
                        <Icon concept="disclose" className={styles.triggerChevron} />
                    </Button>
                    <Menu
                        aria-label="Columns"
                        selectionMode="multiple"
                        defaultSelectedKeys={["name", "rating"]}
                    >
                        <MenuItem id="name">File name</MenuItem>
                        <MenuItem id="rating">Rating</MenuItem>
                        <MenuItem id="date">Capture date</MenuItem>
                        <MenuItem id="camera">Camera</MenuItem>
                        <MenuItem id="lens">Lens</MenuItem>
                    </Menu>
                </MenuTrigger>
            </div>
            <p className={styles.note}>
                One roving list on the shared shell. Automatic markers (submenu chevron / selection check), sugar (icon / shortcut), and a two-line description — never a prop per accessory-combination (§22). Destructive rows carry the data-destructive seam (ink today; the danger tone plugs in at the signals-color hue round). Alignment is menu-scoped: any icon reserves the gutter for every row (see &ldquo;Mixed icons&rdquo; vs &ldquo;View&rdquo;).
            </p>
        </section>
    );
}

const treeFolders: TreeNodeData[] = [
    {
        id: "mac",
        icon: "source",
        count: 48213,
        label: "Macintosh HD",
        children: [
            {
                id: "2024",
                icon: "folder",
                count: 412,
                label: "2024",
                children: [
                    { id: "iceland", icon: "folder", count: 88, label: "Iceland" },
                    { id: "japan", icon: "folder", count: 20300, label: "Japan" },
                    { id: "long", icon: "folder", count: 7, label: "A very long folder name that must end-truncate" },
                ],
            },
            { id: "2023", icon: "folder", count: 1204, label: "2023" },
        ],
    },
];

const treeTags: TreeNodeData[] = [
    {
        id: "places",
        icon: "tag",
        count: 982,
        label: "Places",
        children: [
            { id: "iceland-tag", icon: "tag", count: 88, label: "Iceland" },
            {
                id: "us",
                icon: "tag",
                count: 640,
                label: "United States",
                children: [
                    { id: "portland", icon: "tag", count: 210, label: "Portland" },
                    { id: "seattle", icon: "tag", count: 430, label: "Seattle" },
                ],
            },
        ],
    },
    { id: "film", icon: "tag", count: 53, label: "Shot on film" },
];

const treeCollections: TreeNodeData[] = [
    {
        id: "portfolio",
        icon: "collection",
        count: 64,
        label: "Portfolio",
        children: [
            { id: "best", icon: "collection", count: 24, label: "Best of 2024" },
            { id: "print", icon: "collection", count: 40, label: "Print candidates" },
        ],
    },
    { id: "clients", icon: "collection", count: 318, label: "Clients" },
];

function TreeSpecimens({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Tree — the §12 browser rail (docked chrome; real elbow connectors)</h2>
            <p className={styles.subHead}>
                three modes, one component — Folders (single-select scope), Tags (multi-select union), Collections
            </p>
            <div className={styles.treeSpecimens}>
                <div className={styles.panelSpecimen}>
                    <Tree aria-label="Folders" nodes={treeFolders} defaultExpandedKeys={["mac", "2024"]} defaultSelectedKeys={["iceland"]} />
                </div>
                <div className={styles.panelSpecimen}>
                    <Tree
                        aria-label="Tags"
                        nodes={treeTags}
                        selectionMode="multiple"
                        defaultExpandedKeys={["places", "us"]}
                        defaultSelectedKeys={["iceland-tag", "portland"]}
                    />
                </div>
                <div className={styles.panelSpecimen}>
                    <Tree aria-label="Collections" nodes={treeCollections} defaultExpandedKeys={["portfolio"]} defaultSelectedKeys={["best"]} />
                </div>
            </div>
            <p className={styles.note}>
                Flat docked chrome (no §6 shell), items-driven so the connectors compute from the sibling/last-child
                structure (├ / └). Hierarchy by weight + ink (§9): filled folder kind-icons (§14 tree exception) that
                swap open/closed and ride the tonal ramp (full ink on the active scope, faded elsewhere); the icon is
                the expand toggle, the label sets scope. Rounded floating pill (control radius); sans muted count. Eye-gate
                pins: guide tone/corner, indent step, icon size, pill inset. Deferred to the signals-color round: diff/state hue.
            </p>
        </section>
    );
}

function ControlRowMatrix({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>ControlRow — label + any control, on the control-size height ladder</h2>
            <p className={styles.subHead}>the height ladder (16/20/24/28) — each row hosts a different primitive at its own size</p>
            <div className={styles.panelSpecimen}>
                <ControlRow size="xs" label="Sharpen">
                    <Badge hue="gray">auto</Badge>
                </ControlRow>
                <ControlRow size="sm" label="Watch folder">
                    <Switch aria-label="Watch folder" size="sm" />
                </ControlRow>
                <ControlRow size="md" label="View">
                    <SegmentedControl aria-label="View" defaultValue="loupe" size="md">
                        <Segment id="grid">Grid</Segment>
                        <Segment id="loupe">Loupe</Segment>
                        <Segment id="compare">Compare</Segment>
                    </SegmentedControl>
                </ControlRow>
                <ControlRow size="lg" label="Export">
                    <Button size="lg">Export…</Button>
                </ControlRow>
            </div>
            <p className={styles.subHead}>one row height (md), content of any size — the row never resizes its content</p>
            <div className={styles.panelSpecimen}>
                <ControlRow label="Rating">
                    <Rating value={3} />
                </ControlRow>
                <ControlRow label="Flag">
                    <ToggleButton aria-label="Pick"><Icon concept="flag" /></ToggleButton>
                </ControlRow>
                <ControlRow label="Reject">
                    <Checkbox aria-label="Reject" />
                </ControlRow>
                <ControlRow label="A label long enough that it must end-truncate before it crowds the value">
                    <Badge hue="cyan">RAW</Badge>
                </ControlRow>
                <ControlRow label="Filename">IMG_0421.RAF</ControlRow>
            </div>
            <p className={styles.note}>
                The row owns only its height + its label role; the hosted control brings its own size (no cascade, D33).
                Read-only metadata rows stay on the intent-bound Row below.
            </p>
        </section>
    );
}

function ControlGroupSpecimens({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>ControlGroup — a flush stack of ControlRows sharing one label column</h2>
            <p className={styles.subHead}>uniform size, aligned labels (form style: fixed label column, control fills)</p>
            <div className={styles.panelSpecimen}>
                <ControlGroup>
                    <ControlRow label="View">
                        <SegmentedControl aria-label="View" defaultValue="loupe" size="md">
                            <Segment id="grid">Grid</Segment>
                            <Segment id="loupe">Loupe</Segment>
                            <Segment id="compare">Compare</Segment>
                        </SegmentedControl>
                    </ControlRow>
                    <ControlRow label="Watch folder">
                        <Switch aria-label="Watch folder" size="md" />
                    </ControlRow>
                    <ControlRow label="Sharpen">
                        <Switch aria-label="Sharpen" size="md" defaultSelected />
                    </ControlRow>
                    <ControlRow label="Quality">
                        <Badge hue="gray">auto</Badge>
                    </ControlRow>
                </ControlGroup>
            </div>
            <p className={styles.subHead}>two groups stacked — space between groups, flush within (labelWidth 30%)</p>
            <div className={styles.panelSpecimen}>
                <div className={styles.groupStack}>
                    <ControlGroup labelWidth="30%">
                        <ControlRow label="Rating"><Rating value={3} /></ControlRow>
                        <ControlRow label="Flag">
                            <ToggleButton aria-label="Pick"><Icon concept="flag" /></ToggleButton>
                        </ControlRow>
                    </ControlGroup>
                    <ControlGroup labelWidth="30%">
                        <ControlRow label="ISO">3200</ControlRow>
                        <ControlRow label="Aperture">ƒ/1.8</ControlRow>
                        <ControlRow label="Shutter">1/250 s</ControlRow>
                    </ControlGroup>
                </div>
            </div>
            <p className={styles.subHead}>filled value-rows — the D35 container as a chip list (ControlGroup gap + ControlRow filled)</p>
            <div className={styles.panelSpecimen}>
                <div className={styles.chipList}>
                    <ControlGroup gap>
                        <ControlRow filled label="Salesperson" />
                        <ControlRow filled label="SUM of Units" />
                        <ControlRow filled label="Quarter">Q3</ControlRow>
                    </ControlGroup>
                </div>
            </div>
            <p className={styles.note}>
                The group owns the shared label-column width (labelWidth, default 40%) so rows align; between-group space
                is the parent's gap. Metadata rows stack flush — §8 (space inside rows, not between); filled value-rows
                (D35) read as separate chips in the one control-container material, spaced by the group's gap.
            </p>
        </section>
    );
}

function RowSpecimens({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Row + PanelSection — the §12 grammar, at panel width</h2>
            <div className={styles.panelSpecimen}>
                <PanelSection head="Judgment" intent="control">
                    <Row intent="control" label="Rating" value="unrated" />
                    <Row intent="control" label="Flag">
                        <Button size="md">Pick</Button>
                    </Row>
                </PanelSection>
                <PanelSection head="Collections" intent="list">
                    <Row label="2024 — Iceland" value="1,204" />
                    <Row label="Selects" value="86" />
                    <Row label="A collection with a very long name that must end-truncate" value="12" />
                </PanelSection>
                <PanelSection head="Capture" intent="text">
                    <Row label="ISO" value="400" />
                    <Row label="Aperture" value="ƒ/1.8" />
                    <Row label="Shutter" value="1/250 s" />
                    <Row label="Lens" value="XF 56mm f/1.2 R WR — long enough to truncate and hover-reveal" />
                </PanelSection>
            </div>
        </section>
    );
}

function TypeRoles({ id }: SectionProps) {
    const roles = byPrefix("type-role.");
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Type roles — each is a unit (§9)</h2>
            {roles.map((token) => {
                const roleName = token.path.split(".").at(-1) ?? "";
                return (
                    <div key={token.path} className={styles.roleRow}>
                        <span className={styles.tokenPath}>{roleName}</span>
                        <span className={`alx-type-${roleName}`}>
                            The quick brown fox — 0123456789
                        </span>
                        {token.role !== undefined && <span className={styles.roleNote}>{token.role}</span>}
                    </div>
                );
            })}
        </section>
    );
}

function Swatch({ path }: { path: string }) {
    return <span className={styles.swatch} style={{ background: `var(--alx-${path.replaceAll(".", "-")})` }} />;
}

function ChromeSwatches({ id }: SectionProps) {
    const groups = ["surface.", "cell.", "ink."];
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Chrome roles — live on the active theme</h2>
            {groups.map((prefix) => (
                <div key={prefix} className={styles.swatchRow}>
                    {byPrefix(prefix).map((token) => (
                        <span key={token.path} className={styles.swatchEntry}>
                            <Swatch path={token.path} />
                            <span className={styles.tokenPath}>{token.path}</span>
                        </span>
                    ))}
                </div>
            ))}
        </section>
    );
}

function HueGrid({ id }: SectionProps) {
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>
                Hue scales — accent-eligible: {reference.accentEligible.join(", ")}
            </h2>
            <div className={styles.hueGrid}>
                {hueNames.map((hue) => (
                    <div key={hue} className={styles.hueRow}>
                        <span className={styles.tokenPath}>{hue}</span>
                        {HUE_STEPS.map((step) => (
                            <Swatch key={step} path={`color.${hue}.${step}`} />
                        ))}
                    </div>
                ))}
            </div>
            <p className={styles.note}>steps: {HUE_STEPS.join(" · ")}</p>
        </section>
    );
}

function Scales({ id }: SectionProps) {
    const spaces = byPrefix("space.").sort(
        (first, second) => Number(first.path.split(".")[1]) - Number(second.path.split(".")[1]),
    );
    return (
        <section id={id} className={styles.section}>
            <h2 className={styles.sectionHead}>Space · radius · shadows</h2>
            <div className={styles.swatchRow}>
                {spaces.map((token) => (
                    <span key={token.path} className={styles.swatchEntry}>
                        <span
                            className={styles.spaceBar}
                            style={{ width: `var(--alx-${token.path.replaceAll(".", "-")})` }}
                        />
                        <span className={styles.tokenPath}>{token.path}</span>
                    </span>
                ))}
            </div>
            <div className={styles.swatchRow}>
                {byPrefix("radius.").map((token) => (
                    <span key={token.path} className={styles.swatchEntry}>
                        <span
                            className={styles.radiusChip}
                            style={{ borderRadius: `var(--alx-${token.path.replaceAll(".", "-")})` }}
                        />
                        <span className={styles.tokenPath}>{token.path}</span>
                    </span>
                ))}
                <span className={styles.swatchEntry}>
                    <span className={styles.shadowChip} style={{ boxShadow: "var(--alx-shadow-occlusion)" }} />
                    <span className={styles.tokenPath}>shadow.occlusion</span>
                </span>
            </div>
        </section>
    );
}

// ── Assembly ──────────────────────────────────────────────────────────────────

interface SectionProps {
    id: string;
}

/** ONE source of truth for section order, anchor ids, and the table of contents. */
const SECTIONS: readonly { id: string; label: string; render: (id: string) => ReactNode }[] = [
    { id: "sizing", label: "Sizing system", render: (id) => <SizingSystem id={id} /> },
    { id: "button", label: "Button", render: (id) => <ButtonMatrix id={id} /> },
    { id: "toggle", label: "ToggleButton", render: (id) => <ToggleButtonMatrix id={id} /> },
    { id: "segmented", label: "SegmentedControl", render: (id) => <SegmentedControlSpecimens id={id} /> },
    { id: "textfield", label: "TextField", render: (id) => <TextFieldMatrix id={id} /> },
    { id: "select", label: "Select", render: (id) => <SelectSpecimens id={id} /> },
    { id: "menu", label: "Menu", render: (id) => <MenuSpecimens id={id} /> },
    { id: "tree", label: "Tree", render: (id) => <TreeSpecimens id={id} /> },
    { id: "checkbox", label: "Checkbox", render: (id) => <CheckboxMatrix id={id} /> },
    { id: "switch", label: "Switch", render: (id) => <SwitchMatrix id={id} /> },
    { id: "rating", label: "Rating", render: (id) => <RatingMatrix id={id} /> },
    { id: "badge", label: "Badge", render: (id) => <BadgeMatrix id={id} /> },
    { id: "kbd", label: "Kbd", render: (id) => <KbdMatrix id={id} /> },
    { id: "controlrow", label: "ControlRow", render: (id) => <ControlRowMatrix id={id} /> },
    { id: "controlgroup", label: "ControlGroup", render: (id) => <ControlGroupSpecimens id={id} /> },
    { id: "row", label: "Row + PanelSection", render: (id) => <RowSpecimens id={id} /> },
    { id: "type", label: "Type roles", render: (id) => <TypeRoles id={id} /> },
    { id: "chrome", label: "Chrome roles", render: (id) => <ChromeSwatches id={id} /> },
    { id: "hue", label: "Hue scales", render: (id) => <HueGrid id={id} /> },
    { id: "scales", label: "Space · radius · shadows", render: (id) => <Scales id={id} /> },
];

function TableOfContents() {
    return (
        <nav className={styles.toc} aria-label="Sections">
            {SECTIONS.map((section) => (
                <button
                    key={section.id}
                    type="button"
                    className={styles.tocLink}
                    onClick={() => document.getElementById(section.id)?.scrollIntoView({ behavior: "smooth" })}
                >
                    {section.label}
                </button>
            ))}
        </nav>
    );
}

export function DesignLibrary() {
    return (
        <div className={styles.page}>
            <header className={styles.header}>
                <h1 className={styles.title}>Alexandria design library</h1>
                <ThemeSwitcher />
                <a className={styles.backLink} href="#/">← app shell</a>
            </header>
            <TableOfContents />
            {SECTIONS.map((section) => (
                <Fragment key={section.id}>{section.render(section.id)}</Fragment>
            ))}
        </div>
    );
}
