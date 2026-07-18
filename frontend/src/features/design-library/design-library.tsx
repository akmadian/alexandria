// The in-app design library (#/design-library): specimens and product share one
// implementation (frontend/CLAUDE.md §6). Everything rendered here is driven by
// the compiler's outputs — tokens-reference.json for the inventory, the live
// CSS variables for values — never a hand-listed parallel of either (C15).
//
// ponytail: copy is literal by sanction (task-24 spec) — this is a dev-facing
// surface; C14 keys arrive when product chrome adopts these strings.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Button, type ButtonRung } from "@/components/button/button";
import { Checkbox } from "@/components/checkbox/checkbox";
import { PanelSection } from "@/components/panel-section/panel-section";
import { Row } from "@/components/row/row";
import { Switch } from "@/components/switch/switch";
import { TextField } from "@/components/text-field/text-field";
import { ToggleButton } from "@/components/toggle-button/toggle-button";
import { getTheme, setTheme, themes, type Theme } from "@/lib/theme";
import reference from "@/styles/tokens-reference.json";
import styles from "./design-library.module.css";

const RUNGS = ["ghost", "outline", "tint", "fill", "hero"] as const satisfies readonly ButtonRung[];

const FORCED_STATES = ["rest", "hovered", "pressed", "focus-visible"] as const;
/** "focused" exists for the text-input family, whose ring shows on ANY focus. */
type ForcedState = (typeof FORCED_STATES)[number] | "focused";

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
        <div className={styles.switcher} role="group" aria-label="Theme">
            {themes.map((theme) => (
                <Button
                    key={theme}
                    rung={theme === activeTheme ? "tint" : "ghost"}
                    onPress={() => {
                        setTheme(theme);
                        setActiveTheme(theme);
                    }}
                >
                    {theme}
                </Button>
            ))}
        </div>
    );
}

function ButtonMatrix() {
    const [pressCount, setPressCount] = useState(0);
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>Button — rungs × states</h2>
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
            <p className={styles.note}>
                Sizes: <Button size="control">control 24</Button>{" "}
                <Button size="control-lg">control-lg 28</Button>{" "}
                <span className={styles.pressReadout}>{pressCount > 0 ? `${pressCount} presses` : ""}</span>
            </p>
        </section>
    );
}

function ToggleButtonMatrix() {
    const specimens: { name: string; state: ForcedState; selected: boolean; disabled?: boolean }[] = [
        { name: "rest", state: "rest", selected: false },
        { name: "hovered", state: "hovered", selected: false },
        { name: "pressed", state: "pressed", selected: false },
        { name: "focus-visible", state: "focus-visible", selected: false },
        { name: "selected", state: "rest", selected: true },
        { name: "selected+hovered", state: "hovered", selected: true },
        { name: "disabled", state: "rest", selected: false, disabled: true },
        { name: "disabled on", state: "rest", selected: true, disabled: true },
    ];
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>ToggleButton — the boolean register (§14: on = fill)</h2>
            <div className={styles.swatchRow}>
                {specimens.map((specimen) => (
                    <span key={specimen.name} className={styles.swatchEntry}>
                        <span className={styles.matrixLabel}>{specimen.name}</span>
                        <ForcedStateCell state={specimen.state}>
                            <ToggleButton defaultSelected={specimen.selected} isDisabled={specimen.disabled}>
                                Raw
                            </ToggleButton>
                        </ForcedStateCell>
                    </span>
                ))}
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>live</span>
                    <ToggleButton>Raw</ToggleButton>
                </span>
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>control-lg</span>
                    <ToggleButton size="control-lg" defaultSelected>Raw</ToggleButton>
                </span>
            </div>
        </section>
    );
}

function CheckboxMatrix() {
    const specimens: {
        name: string;
        state: ForcedState;
        checked?: boolean;
        mixed?: boolean;
        disabled?: boolean;
        invalid?: boolean;
    }[] = [
        { name: "rest", state: "rest" },
        { name: "hovered", state: "hovered" },
        { name: "pressed", state: "pressed" },
        { name: "focus-visible", state: "focus-visible" },
        { name: "checked", state: "rest", checked: true },
        { name: "checked+hovered", state: "hovered", checked: true },
        { name: "mixed", state: "rest", mixed: true },
        { name: "invalid", state: "rest", invalid: true },
        { name: "disabled", state: "rest", disabled: true },
        { name: "disabled checked", state: "rest", checked: true, disabled: true },
    ];
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>Checkbox — the toggles-on ledger row (§5)</h2>
            <div className={styles.swatchRow}>
                {specimens.map((specimen) => (
                    <span key={specimen.name} className={styles.swatchEntry}>
                        <span className={styles.matrixLabel}>{specimen.name}</span>
                        <ForcedStateCell state={specimen.state}>
                            <Checkbox
                                defaultSelected={specimen.checked}
                                isIndeterminate={specimen.mixed}
                                isDisabled={specimen.disabled}
                                isInvalid={specimen.invalid}
                            >
                                Reject
                            </Checkbox>
                        </ForcedStateCell>
                    </span>
                ))}
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>live</span>
                    <Checkbox>Reject</Checkbox>
                </span>
            </div>
        </section>
    );
}

function SwitchMatrix() {
    const specimens: {
        name: string;
        state: ForcedState;
        on?: boolean;
        disabled?: boolean;
    }[] = [
        { name: "rest", state: "rest" },
        { name: "hovered", state: "hovered" },
        { name: "pressed", state: "pressed" },
        { name: "focus-visible", state: "focus-visible" },
        { name: "on", state: "rest", on: true },
        { name: "on+hovered", state: "hovered", on: true },
        { name: "disabled", state: "rest", disabled: true },
        { name: "disabled on", state: "rest", on: true, disabled: true },
    ];
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>Switch — the immediate-effect boolean (§5)</h2>
            <div className={styles.swatchRow}>
                {specimens.map((specimen) => (
                    <span key={specimen.name} className={styles.swatchEntry}>
                        <span className={styles.matrixLabel}>{specimen.name}</span>
                        <ForcedStateCell state={specimen.state}>
                            <Switch defaultSelected={specimen.on} isDisabled={specimen.disabled}>
                                Watch
                            </Switch>
                        </ForcedStateCell>
                    </span>
                ))}
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>live</span>
                    <Switch>Watch</Switch>
                </span>
            </div>
        </section>
    );
}

function TextFieldMatrix() {
    const specimens: {
        name: string;
        state: ForcedState;
        invalid?: boolean;
        disabled?: boolean;
        description?: string;
    }[] = [
        { name: "rest", state: "rest" },
        { name: "hovered", state: "hovered" },
        { name: "focused", state: "focused" },
        { name: "invalid", state: "rest", invalid: true },
        { name: "described", state: "rest", description: "Shown in the panel tree" },
        { name: "disabled", state: "rest", disabled: true },
    ];
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>TextField — the field composite (§25)</h2>
            <div className={styles.swatchRow}>
                {specimens.map((specimen) => (
                    <span key={specimen.name} className={styles.swatchEntry}>
                        <span className={styles.matrixLabel}>{specimen.name}</span>
                        <ForcedStateCell state={specimen.state}>
                            <TextField
                                label="Collection"
                                defaultValue="2024 — Iceland"
                                description={specimen.description}
                                errorMessage="Name is taken"
                                isInvalid={specimen.invalid}
                                isDisabled={specimen.disabled}
                            />
                        </ForcedStateCell>
                    </span>
                ))}
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>live</span>
                    <TextField label="Collection" placeholder="Type here…" />
                </span>
                <span className={styles.swatchEntry}>
                    <span className={styles.matrixLabel}>control-lg</span>
                    <TextField label="Collection" size="control-lg" defaultValue="Selects" />
                </span>
            </div>
        </section>
    );
}

function RowSpecimens() {
    return (
        <section className={styles.section}>
            <h2 className={styles.sectionHead}>Row + PanelSection — the §12 grammar, at panel width</h2>
            <div className={styles.panelSpecimen}>
                <PanelSection head="Judgment" intent="control">
                    <Row intent="control" label="Rating" value="unrated" />
                    <Row intent="control" label="Flag">
                        <Button size="control">Pick</Button>
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

function TypeRoles() {
    const roles = byPrefix("type-role.");
    return (
        <section className={styles.section}>
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

function ChromeSwatches() {
    const groups = ["surface.", "cell.", "ink."];
    return (
        <section className={styles.section}>
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

function HueGrid() {
    return (
        <section className={styles.section}>
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

function Scales() {
    const spaces = byPrefix("space.").sort(
        (first, second) => Number(first.path.split(".")[1]) - Number(second.path.split(".")[1]),
    );
    return (
        <section className={styles.section}>
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

export function DesignLibrary() {
    return (
        <div className={styles.page}>
            <header className={styles.header}>
                <h1 className={styles.title}>Alexandria design library</h1>
                <ThemeSwitcher />
                <a className={styles.backLink} href="#/">← app shell</a>
            </header>
            <ButtonMatrix />
            <ToggleButtonMatrix />
            <CheckboxMatrix />
            <SwitchMatrix />
            <TextFieldMatrix />
            <RowSpecimens />
            <TypeRoles />
            <ChromeSwatches />
            <HueGrid />
            <Scales />
        </div>
    );
}
