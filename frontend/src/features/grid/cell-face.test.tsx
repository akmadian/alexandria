// CellFace pins the row-fed contract: a pure projection of AssetRow. The
// configurable header resolves per field; flag/label/badge are silent until
// informative (§10); the rating slot always shows the readout — unrated
// renders dim hollow positions (ratified by eye 2026-07-19).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import type { AssetRow } from "@/api/contract";
import { CellFace } from "./cell-face";

function row(overrides: Partial<AssetRow> = {}): AssetRow {
    return {
        kind: "asset",
        id: "asset-1",
        sourceId: "source-1",
        filename: "photo.jpg",
        fileType: "image",
        fileStatus: "online",
        rating: null,
        colorLabel: null,
        flag: null,
        width: 7728,
        height: 5152,
        durationSecs: null,
        cameraModel: "A7 IV",
        capturedAt: null,
        ingestedAt: "2026-07-01T00:00:00Z",
        thumbnailAt: null,
        relativePath: "photo.jpg",
        sizeBytes: 1000,
        thumbURL: "data:image/svg+xml,x",
        ...overrides,
    };
}

test("resolves the default header from the row: index, filename, dimensions, camera", () => {
    render(<CellFace row={row()} index={148} />);
    expect(screen.getByText("149")).toBeInTheDocument();
    expect(screen.getByText("photo.jpg")).toBeInTheDocument();
    expect(screen.getByText("7728 × 5152")).toBeInTheDocument();
    expect(screen.getByText("A7 IV")).toBeInTheDocument();
});

test("header slots are configurable; none renders an empty slot", () => {
    render(<CellFace row={row()} index={0} header={["filename", "none", "none", "none"]} />);
    expect(screen.getByText("photo.jpg")).toBeInTheDocument();
    expect(screen.queryByText("A7 IV")).toBeNull();
});

test("capturedAt and size header fields format at the render edge via Intl", () => {
    render(
        <CellFace
            row={row({ capturedAt: "2026-06-14T12:00:00Z" })}
            index={0}
            header={["capturedAt", "size", "none", "none"]}
        />,
    );
    expect(screen.getByText("Jun 14, 2026")).toBeInTheDocument();
    expect(screen.getByText("1 kB")).toBeInTheDocument();
});

test("the thumbnail wires src and alt straight from the row", () => {
    render(<CellFace row={row()} index={0} />);
    const image = screen.getByRole("img", { name: "photo.jpg" });
    expect(image).toHaveAttribute("src", "data:image/svg+xml,x");
});

test("the rating slot always shows the readout: unrated reads dim-hollow, rated reads filled", () => {
    const unrated = render(<CellFace row={row()} index={0} />);
    expect(unrated.getByLabelText("Unrated")).toBeInTheDocument();
    unrated.unmount();
    render(<CellFace row={row({ rating: 3 })} index={0} />);
    expect(screen.getByLabelText("Rated 3")).toBeInTheDocument();
});

test("flag marks: pick and reject are distinct; null is silent", () => {
    const pick = render(<CellFace row={row({ flag: "pick" })} index={0} />);
    expect(pick.getByLabelText("Pick")).toBeInTheDocument();
    pick.unmount();
    const reject = render(<CellFace row={row({ flag: "reject" })} index={0} />);
    expect(reject.getByLabelText("Reject")).toBeInTheDocument();
    reject.unmount();
    const none = render(<CellFace row={row()} index={0} />);
    expect(none.queryByLabelText("Pick")).toBeNull();
    expect(none.queryByLabelText("Reject")).toBeNull();
});

test("the label swatch carries its §5 hue and an a11y name; null is silent", () => {
    const labeled = render(<CellFace row={row({ colorLabel: "red" })} index={0} />);
    const swatch = labeled.getByLabelText("Red label");
    expect(swatch.getAttribute("style")).toContain("var(--alx-label-red)");
    labeled.unmount();
    const plain = render(<CellFace row={row()} index={0} />);
    expect(plain.queryByLabelText(/label$/)).toBeNull();
});

test("the type badge is silent for the baseline image and shown for the rest", () => {
    const silent = render(<CellFace row={row()} index={0} />);
    expect(silent.queryByText("RAW")).toBeNull();
    silent.unmount();
    render(<CellFace row={row({ fileType: "raw" })} index={0} />);
    expect(screen.getByText("RAW")).toBeInTheDocument();
});
