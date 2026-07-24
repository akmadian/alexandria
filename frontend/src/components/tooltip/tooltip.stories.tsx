import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/button/button";
import { Tooltip, TooltipTrigger, type TooltipVariant } from "./tooltip";

const VARIANTS: TooltipVariant[] = ["dark", "inverse"];

const meta = {
    title: "Primitives/Tooltip",
    component: Tooltip,
    args: { children: "Rotate 90° left", variant: "dark" },
    argTypes: {
        variant: { control: "inline-radio", options: VARIANTS },
        children: { control: "text" },
    },
} satisfies Meta<typeof Tooltip>;

export default meta;

type Story = StoryObj<typeof meta>;

const stage = {
    display: "flex",
    gap: "var(--alx-space-8)",
    padding: "var(--alx-space-8)",
    alignItems: "center",
    justifyContent: "center",
} as const;

// Interactive: hover or focus the button (warmup ~700ms); drive variant + label from Controls.
export const Playground: Story = {
    render: (args) => (
        <div style={stage}>
            <TooltipTrigger>
                <Button>Hover or focus me</Button>
                <Tooltip {...args} />
            </TooltipTrigger>
        </div>
    ),
};

// Both variants forced open (placement bottom for room), so polarity reads at a glance. Switch
// the Storybook theme toolbar: `dark` holds a fixed dark chip on every theme; `inverse` flips to
// a light chip on the dark themes (graphite/carbon), built from each theme's own poles.
export const Variants: Story = {
    render: () => (
        <div style={stage}>
            {VARIANTS.map((variant) => (
                <TooltipTrigger key={variant} isOpen>
                    <Button>{variant}</Button>
                    <Tooltip variant={variant} placement="bottom">
                        Rotate 90° left
                    </Tooltip>
                </TooltipTrigger>
            ))}
        </div>
    ),
};
