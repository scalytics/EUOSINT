import type { Alert } from "@/types/alert";
import { AlertDetail } from "@/components/AlertDetail";
import { useBottomSheet } from "./useBottomSheet";
import { useEffect } from "react";

interface Props {
  alert: Alert | null;
  onClose: () => void;
}

export function MobileAlertSheet({ alert, onClose }: Props) {
  const { isOpen, open, close, sheetRef, onDragStart, onDragMove, onDragEnd } =
    useBottomSheet();

  useEffect(() => {
    if (alert) {
      open();
    }
  }, [alert, open]);

  function handleClose() {
    close();
    // Delay onClose so the animation plays
    setTimeout(onClose, 300);
  }

  if (!isOpen || !alert) return null;

  return (
    <>
      <div className="mobile-sheet-backdrop" onClick={handleClose} />
      <div
        ref={sheetRef}
        className="mobile-sheet"
        style={{ transform: "translateY(0)" }}
      >
        <div
          className="mobile-sheet-handle"
          onTouchStart={onDragStart}
          onTouchMove={onDragMove}
          onTouchEnd={onDragEnd}
        />
        <div className="mobile-sheet-content">
          <AlertDetail alert={alert} onClose={handleClose} />
        </div>
      </div>
    </>
  );
}
