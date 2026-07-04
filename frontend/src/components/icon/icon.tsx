import type { LucideIcon } from "lucide-react";

// One door for icons: size/stroke are fixed to the design system so call sites
// can't drift. Decorative by default (aria-hidden); pass a label to make one meaningful.
interface IconProps {
    icon: LucideIcon;
    size?: 12 | 14 | 16 | 20;
    label?: string;
    className?: string;
}

export const Icon = ({ icon: Glyph, size = 16, label, className }: IconProps) => (
    <Glyph size={size} strokeWidth={1.5} aria-hidden={label ? undefined : true} aria-label={label} className={className} />
);
