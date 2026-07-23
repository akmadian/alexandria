import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Button, type ButtonSize } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import { cssVar, find, Mono, Page, rawValue, Section } from "./specimens";

// The proportional control-size ladder (D33): four rungs where height, inline
// padding, label type role, and icon size all step in lockstep. 24px is the
// pointer hit-target floor. Sizes are set explicitly per control — no cascade —
// so the row-rhythm below is usage discipline, not automatic.

const px = (path: string) => rawValue(find(path));
const iconSizeVar = (path: string): CSSProperties => ({ ["--alx-size-icon"]: `var(${cssVar(path)})` }) as CSSProperties;

interface Rung {
    size: ButtonSize;
    height: string; // token path → the control height
    pad: string; // token path → inline padding (button pads 0 space-N)
    typeRole: string; // display name of the paired control-text role
    typeClass: string; // the emitted .alx-type-* unit class
    icon: string; // token path → the paired icon mark
    hit: string; // one word: below / meets / above the 24px floor
    hitNote: string;
    usage: string;
}

const LADDER: Rung[] = [
    { size: "xs", height: "size.control-xs", pad: "space.1", typeRole: "control-xs", typeClass: "alx-type-control-xs", icon: "size.icon-xs", hit: "below", hitNote: "mouse-only sub-floor", usage: "Inline-edit / inspector-dense tier" },
    { size: "sm", height: "size.control-sm", pad: "space.2", typeRole: "control-sm", typeClass: "alx-type-control-sm", icon: "size.icon-sm", hit: "below", hitNote: "hit area expands to the 24px floor", usage: "Dense inline fields + their triggers" },
    { size: "md", height: "size.control-md", pad: "space.3", typeRole: "control", typeClass: "alx-type-control", icon: "size.icon", hit: "meets", hitNote: "sits exactly on the floor", usage: "THE default control height" },
    { size: "lg", height: "size.control-lg", pad: "space.4", typeRole: "control-lg", typeClass: "alx-type-control-lg", icon: "size.icon-lg", hit: "above", hitNote: "clears the floor", usage: "Dialog CTAs, hero spots" },
];

// D33 row-rhythm: an interactive control sits ONE rung below its row (2px breathe
// each side); read-only text rows host no control.
const RHYTHM: { row: string; rowHeight: string; controlHeight: string | null; note: string }[] = [
    { row: "control", rowHeight: "size.row-control", controlHeight: "size.control-md", note: "hosts a control-md (24) — controls breathe" },
    { row: "list", rowHeight: "size.row-list", controlHeight: "size.control-sm", note: "hosts a control-sm (20) — sits on the hit-target floor" },
    { row: "text", rowHeight: "size.row-text", controlHeight: null, note: "read-only (metadata / EXIF) — the instrument voice" },
];

const cell: CSSProperties = { padding: "var(--alx-space-2) var(--alx-space-6) var(--alx-space-2) 0", verticalAlign: "top", whiteSpace: "nowrap" };

function LadderTable() {
    return (
        <div style={{ overflowX: "auto" }}>
            <table style={{ borderCollapse: "collapse", width: "100%", minWidth: 760 }}>
                <thead>
                    <tr>
                        {["Level", "Height", "Inline pad", "Type role", "Icon", "Hit target", "Usage"].map((head) => (
                            <th key={head} className="alx-type-caption" style={{ ...cell, textAlign: "left", color: "var(--alx-ink-3)", fontWeight: 400 }}>
                                {head}
                            </th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {LADDER.map((rung) => (
                        <tr key={rung.size} style={{ borderTop: "1px solid var(--alx-ink-hairline)" }}>
                            <td style={cell}>
                                <span className="alx-type-label-sm">{rung.size}</span>
                            </td>
                            <td style={cell}>
                                <Mono>{px(rung.height)}</Mono>
                            </td>
                            <td style={cell}>
                                <Mono>{px(rung.pad)}</Mono>
                            </td>
                            <td style={cell}>
                                <span className={rung.typeClass}>Rename</span> <span className="alx-type-caption" style={{ color: "var(--alx-ink-4)" }}>.{rung.typeClass}</span>
                            </td>
                            <td style={cell}>
                                <Mono>{px(rung.icon)}</Mono>
                            </td>
                            <td style={{ ...cell, whiteSpace: "normal" }}>
                                <span className="alx-type-caption">
                                    {rung.hit} — {rung.hitNote}
                                </span>
                            </td>
                            <td style={{ ...cell, whiteSpace: "normal" }}>
                                <span className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                                    {rung.usage}
                                </span>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

function ProportionalControls() {
    return (
        <div style={{ display: "flex", gap: "var(--alx-space-8)", alignItems: "flex-end", flexWrap: "wrap" }}>
            {LADDER.map((rung) => (
                <div key={rung.size} style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-2)", alignItems: "center" }}>
                    <Button size={rung.size} rung="outline">
                        <Icon concept="settings" />
                        Rename
                    </Button>
                    <span className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                        {rung.size} · {px(rung.height)}
                    </span>
                </div>
            ))}
        </div>
    );
}

function HitTargetFloor() {
    // Bars share one bottom baseline; the dashed line sits exactly hit-target (24px)
    // above it, so xs/sm fall short, md meets it, lg clears it. Labels ride below in
    // a parallel row of the same column widths so they stay aligned under each bar.
    return (
        <div style={{ maxWidth: 520 }}>
            <div style={{ position: "relative", display: "flex", gap: "var(--alx-space-8)", alignItems: "flex-end", height: 40 }}>
                <div style={{ position: "absolute", left: 0, right: 0, bottom: `var(${cssVar("size.hit-target")})`, borderTop: "1px dashed var(--alx-attention)", pointerEvents: "none" }}>
                    <span className="alx-type-caption" style={{ position: "absolute", right: 0, top: "calc(-1 * var(--alx-space-4))", color: "var(--alx-attention)" }}>
                        {px("size.hit-target")} hit-target floor
                    </span>
                </div>
                {LADDER.map((rung) => (
                    <div key={rung.size} style={{ width: 48, height: `var(${cssVar(rung.height)})`, background: "var(--alx-accent)", borderRadius: "var(--alx-radius-control, 6px)" }} />
                ))}
            </div>
            <div style={{ display: "flex", gap: "var(--alx-space-8)", marginTop: "var(--alx-space-1)" }}>
                {LADDER.map((rung) => (
                    <div key={rung.size} style={{ width: 48, textAlign: "center" }}>
                        <div className="alx-type-caption">
                            {rung.size} · {px(rung.height)}
                        </div>
                        <div className="alx-type-caption" style={{ color: "var(--alx-ink-4)" }}>
                            {rung.hit}
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}

function IconRamp() {
    return (
        <div style={{ display: "flex", gap: "var(--alx-space-8)", alignItems: "flex-end" }}>
            {LADDER.map((rung) => (
                <div key={rung.size} style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "var(--alx-space-1)" }}>
                    <span style={{ ...iconSizeVar(rung.icon), display: "inline-flex", color: "var(--alx-ink-1)" }}>
                        <Icon concept="settings" />
                    </span>
                    <span className="alx-type-caption">{px(rung.icon)}</span>
                    <span className="alx-type-caption" style={{ color: "var(--alx-ink-4)" }}>
                        {rung.size}
                    </span>
                </div>
            ))}
        </div>
    );
}

function RowRhythm() {
    return (
        <div style={{ display: "grid", gap: "var(--alx-space-3)" }}>
            {RHYTHM.map((mapping) => (
                <div key={mapping.row} style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
                    <span className="alx-type-label-sm" style={{ width: 64 }}>
                        {mapping.row}
                    </span>
                    <div
                        style={{
                            width: 220,
                            height: `var(${cssVar(mapping.rowHeight)})`,
                            background: "var(--alx-surface-panel)",
                            boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)",
                            borderRadius: 2,
                            display: "flex",
                            alignItems: "center",
                            padding: "0 var(--alx-space-3)",
                        }}
                    >
                        {mapping.controlHeight ? (
                            <div style={{ height: `var(${cssVar(mapping.controlHeight)})`, width: 88, background: "var(--alx-accent)", borderRadius: "var(--alx-radius-control, 6px)" }} />
                        ) : (
                            <span className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                                read-only
                            </span>
                        )}
                    </div>
                    <span className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                        {px(mapping.rowHeight)} row · {mapping.note}
                    </span>
                </div>
            ))}
        </div>
    );
}

const meta: Meta = {
    title: "Design System/Control Sizes",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const ControlSizes: Story = {
    render: () => (
        <Page
            title="Control Sizes"
            intro="A four-rung proportional ladder (D33): height 16 / 20 / 24 / 28 — and inline padding, label type role, and icon size all step with it. 24px is the pointer hit-target floor. Size is set explicitly per control (no cascade)."
        >
            <Section title="The ladder" hint="every dimension steps in lockstep">
                <LadderTable />
            </Section>
            <Section title="Proportional controls" hint="height · padding · label · icon scale together">
                <ProportionalControls />
            </Section>
            <Section title="The hit-target floor" hint="24px pointer comfort minimum">
                <HitTargetFloor />
            </Section>
            <Section title="Icon ramp" hint="12 / 14 / 16 / 18 — one mark per rung (§14)">
                <IconRamp />
            </Section>
            <Section title="Usage — the row rhythm (D33)" hint="a control sits one rung below its row">
                <RowRhythm />
            </Section>
        </Page>
    ),
};
