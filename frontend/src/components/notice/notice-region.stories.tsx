import { useEffect } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/button/button";
import "@/i18n"; // resolves the notice messageKey (an i18n key) at render (C14)
import { pushNotice, useNotices } from "@/stores/notices";
import { NoticeRegion } from "./notice-region";

// NoticeRegion is the visible half of the loud-failure floor (stores/notices.ts):
// it renders the live notices from the global store as dismissible messages, or
// null when empty. Notices auto-expire after 6s, so this specimen keeps one alive
// on an interval and offers a button to raise more — the region is inline (no
// positioning grammar by doctrine), so it appears right here, not at a corner.
const meta = {
    title: "Primitives/NoticeRegion",
    component: NoticeRegion,
} satisfies Meta<typeof NoticeRegion>;

export default meta;

type Story = StoryObj<typeof meta>;

function NoticeDemo() {
    const notices = useNotices();
    useEffect(() => {
        pushNotice("errors.writeFailed");
        // Re-raise before the 6s expiry so the specimen always shows a live notice.
        const timer = setInterval(() => pushNotice("errors.writeFailed"), 4000);
        return () => clearInterval(timer);
    }, []);
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-3)", alignItems: "flex-start" }}>
            <span className="alx-type-caption">
                A failed write pushes a notice; the region renders it as a dismissible message ({notices.length} live). Loud failure, never silent.
            </span>
            <Button rung="outline" onPress={() => pushNotice("errors.writeFailed")}>
                Raise notice
            </Button>
            <NoticeRegion />
        </div>
    );
}

export const Playground: Story = {
    render: () => <NoticeDemo />,
};
