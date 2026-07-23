import { useEffect } from "react";
import type { Preview } from "@storybook/react-vite";

// The preview iframe has none of the app's boot, so the design system must be
// loaded by hand: emitted tokens first (they define every --alx-* variable),
// then app-base (fonts + body ground). Without these the canvas renders unstyled.
import "../src/styles/tokens.css";
import "../src/styles/app-base.css";

// The four ratified themes (tokens-reference.json → themes). Switched at runtime
// via [data-theme] on the root, the same lever the app uses.
const THEMES = ["paper", "linen", "graphite", "carbon"] as const;

const preview: Preview = {
    parameters: {
        controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
        // Foundations lead; primitives follow the §6 build ladder (leaves first),
        // not the default alphabetical order. Anything unlisted falls in after,
        // alphabetically. Stories within a component keep file order (Playground → Matrix).
        options: {
            storySort: {
                order: [
                    "Design System",
                    ["Colors", "Typography", "Spacing & Sizing", "Radius", "Elevation", "Motion", "Reference"],
                    "Primitives",
                    ["Button", "ToggleButton", "Checkbox", "Switch", "SegmentedControl", "TextField", "Rating", "Badge", "Icon", "LabelSwatch", "Row", "ControlRow", "ControlGroup", "PanelSection", "NoticeRegion"],
                ],
            },
        },
    },
    initialGlobals: { theme: "paper" },
    globalTypes: {
        theme: {
            description: "Design-system theme",
            toolbar: {
                title: "Theme",
                icon: "paintbrush",
                items: THEMES.map((value) => ({ value, title: value })),
                dynamicTitle: true,
            },
        },
    },
    decorators: [
        (Story, { globals }) => {
            useEffect(() => {
                document.documentElement.setAttribute("data-theme", globals.theme ?? "paper");
            }, [globals.theme]);
            return <Story />;
        },
    ],
};

export default preview;
