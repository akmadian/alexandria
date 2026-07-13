// The design library — a living specimen page (design-constitution §22/§30
// tooling). Renders the REAL primitives from components/ against the injected
// token variables: if it looks right here, it is right everywhere, because the
// specimen and the product share one implementation and one token source.
//
// Frozen state columns re-render the primitive's own markup + CSS module with
// forced RAC data-attributes (RAC only sets them on real interaction); the
// "live" column is the actual interactive component.
//
// ponytail: internal dev surface — display strings are literals, exempt from
// C14 like the playground scaffolding.

import { Check, ChevronDown } from "lucide-react";
import { useState } from "react";
import { Button, type ButtonRung } from "@/components/button/button";
import buttonStyles from "@/components/button/button.module.css";
import popoverStyles from "@/components/popover/popover.module.css";
import { Select, SelectItem } from "@/components/select/select";
import selectStyles from "@/components/select/select.module.css";
import { cx } from "@/lib/cx";
import { currentTheme, setTheme, THEMES, type ThemeName } from "@/styles/tokens";
import s from "./design-library.module.css";

const RUNGS: ButtonRung[] = ["ghost", "outline", "tint", "fill", "hero"];

const FORCED_STATES: Array<[label: string, attrs: Record<string, string>]> = [
    ["rest", {}],
    ["hovered", { "data-hovered": "" }],
    ["pressed", { "data-hovered": "", "data-pressed": "" }],
    ["focus", { "data-focus-visible": "" }],
    ["disabled", { "data-disabled": "" }],
];

const SURFACE_VARS = [
    "--alx-surface-panel", "--alx-surface-field", "--alx-surface-hover",
    "--alx-surface-pressed", "--alx-surface-selected",
    "--alx-cell-well", "--alx-cell-rest", "--alx-cell-hover",
    "--alx-cell-selected", "--alx-cell-active",
    "--alx-ink-1", "--alx-ink-2", "--alx-ink-3", "--alx-ink-4", "--alx-ink-hairline",
];

function ButtonMatrix() {
    return (
        <table className={s.matrix}>
            <thead>
                <tr>
                    <th>rung</th>
                    {FORCED_STATES.map(([label]) => <th key={label}>{label}</th>)}
                    <th>live</th>
                </tr>
            </thead>
            <tbody>
                {RUNGS.map((rung) => (
                    <tr key={rung}>
                        <td className={s.rowLabel}><code>rung="{rung}"</code></td>
                        {FORCED_STATES.map(([label, attrs]) => (
                            <td key={label}>
                                <span className={cx(buttonStyles.button, buttonStyles[rung])} {...attrs}>
                                    Button
                                </span>
                            </td>
                        ))}
                        <td>
                            <Button rung={rung}>Button</Button>
                        </td>
                    </tr>
                ))}
            </tbody>
        </table>
    );
}

// Trigger states use aria-expanded for "open" — that's the attribute the real
// RAC trigger carries, and the one .trigger styles against.
const TRIGGER_STATES: Array<[label: string, attrs: Record<string, string>]> = [
    ["rest", {}],
    ["hovered", { "data-hovered": "" }],
    ["open", { "aria-expanded": "true" }],
    ["focus", { "data-focus-visible": "" }],
    ["disabled", { "data-disabled": "" }],
];

function FrozenSelectItem({ label, attrs, selected }: { label: string; attrs?: Record<string, string>; selected?: boolean }) {
    return (
        <span className={selectStyles.item} {...attrs}>
            <span className={selectStyles.check}>{selected ? <Check size={14} /> : null}</span>
            <span className={selectStyles.label}>{label}</span>
        </span>
    );
}

function SelectMatrix() {
    return (
        <>
            <table className={s.matrix}>
                <thead>
                    <tr>
                        <th>part</th>
                        {TRIGGER_STATES.map(([label]) => <th key={label}>{label}</th>)}
                        <th>live</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td className={s.rowLabel}><code>trigger</code></td>
                        {TRIGGER_STATES.map(([label, attrs]) => (
                            <td key={label}>
                                <span
                                    className={cx(buttonStyles.button, buttonStyles.outline, selectStyles.trigger)}
                                    {...attrs}
                                >
                                    <span className={selectStyles.value}>value</span>
                                    <ChevronDown size={13} className={selectStyles.chevron} />
                                </span>
                            </td>
                        ))}
                        <td>
                            <Select aria-label="Specimen" defaultSelectedKey="one">
                                <SelectItem id="one">Option one</SelectItem>
                                <SelectItem id="two">Option two</SelectItem>
                                <SelectItem id="three" isDisabled>Option three</SelectItem>
                            </Select>
                        </td>
                    </tr>
                </tbody>
            </table>
            <div className={s.popoverSpecimen}>
                <div className={popoverStyles.popover}>
                    <FrozenSelectItem label="rest" />
                    <FrozenSelectItem label="focused" attrs={{ "data-focused": "" }} />
                    <FrozenSelectItem label="pressed" attrs={{ "data-pressed": "" }} />
                    <FrozenSelectItem label="selected" selected />
                    <FrozenSelectItem label="disabled" attrs={{ "data-disabled": "" }} />
                </div>
                <p className={s.specimenNote}>
                    The §6 transient surface, frozen open: theme-following panel, hairline,
                    occlusion shadow, <code>--alx-r-transient</code>. Item states left to right
                    in the list.
                </p>
            </div>
        </>
    );
}

function SwatchStrip() {
    return (
        <div className={s.swatches}>
            {SURFACE_VARS.map((name) => (
                <button
                    key={name}
                    type="button"
                    className={s.swatch}
                    onClick={() => navigator.clipboard?.writeText(`var(${name})`)}
                    title={`copy var(${name})`}
                >
                    <span className={s.chip} style={{ background: `var(${name})` }} />
                    <code>{name}</code>
                </button>
            ))}
        </div>
    );
}

export function DesignLibrary() {
    // Dogfooding: the library's own theme control is the Select primitive.
    const [theme, setThemeState] = useState<ThemeName>(currentTheme);
    return (
        <div className={s.page}>
            <header className={s.head}>
                <div>
                    <h1 className={s.title}>Design library</h1>
                    <p className={s.sub}>
                        Real primitives, real tokens (<code>design/tokens.json</code>). Swatch/contract
                        deep-dive: the static library at <code>design/library/</code> (port 8123).
                    </p>
                </div>
                <span className={s.themeControl}>
                    theme
                    <Select
                        aria-label="Theme"
                        selectedKey={theme}
                        onSelectionChange={(key) => {
                            setTheme(key as ThemeName);
                            setThemeState(key as ThemeName);
                        }}
                    >
                        {THEMES.map((t) => <SelectItem key={t} id={t}>{t}</SelectItem>)}
                    </Select>
                </span>
            </header>

            <section>
                <h2 className={s.sectionTitle}>Button <span className={s.ref}>§4 §7 §17</span></h2>
                <ButtonMatrix />
            </section>

            <section>
                <h2 className={s.sectionTitle}>Select <span className={s.ref}>§6 §7 §24</span></h2>
                <SelectMatrix />
            </section>

            <section>
                <h2 className={s.sectionTitle}>Surfaces &amp; inks <span className={s.ref}>§7 §20</span></h2>
                <SwatchStrip />
            </section>
        </div>
    );
}
