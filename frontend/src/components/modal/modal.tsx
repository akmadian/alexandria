import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Dialog as AriaDialog, Heading as AriaHeading, Modal as AriaModal, ModalOverlay as AriaModalOverlay } from "react-aria-components";
import { useTranslation } from "react-i18next";
import { cx } from "@/lib/cx";
import { Button } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import s from "./modal.module.css";

interface ModalProps {
    title: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    size?: "sm" | "md" | "lg";
    children: ReactNode;
}

export const Modal = ({ title, isOpen, onOpenChange, size = "md", children }: ModalProps) => {
    const { t } = useTranslation();
    return (
        <AriaModalOverlay className={s.overlay} isOpen={isOpen} onOpenChange={onOpenChange} isDismissable>
            <AriaModal className={cx(s.modal, s[size])}>
                <AriaDialog className={s.dialog}>
                    {({ close }) => (
                        <>
                            <header className={s.header}>
                                <AriaHeading slot="title" className={s.title}>
                                    {title}
                                </AriaHeading>
                                <Button size="sm" onPress={close} aria-label={t("modal.close")}>
                                    <Icon icon={X} size={14} />
                                </Button>
                            </header>
                            <div className={s.body}>{children}</div>
                        </>
                    )}
                </AriaDialog>
            </AriaModal>
        </AriaModalOverlay>
    );
};
