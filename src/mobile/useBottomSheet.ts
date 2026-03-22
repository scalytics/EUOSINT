import { useRef, useState, useCallback } from "react";

export function useBottomSheet() {
  const [isOpen, setIsOpen] = useState(false);
  const sheetRef = useRef<HTMLDivElement>(null);
  const startY = useRef(0);
  const currentTranslate = useRef(0);
  const dragging = useRef(false);

  const open = useCallback(() => {
    setIsOpen(true);
    currentTranslate.current = 0;
  }, []);

  const close = useCallback(() => {
    const el = sheetRef.current;
    if (el) {
      el.style.transform = "translateY(100%)";
    }
    setTimeout(() => {
      setIsOpen(false);
      currentTranslate.current = 0;
    }, 300);
  }, []);

  const onDragStart = useCallback((e: React.TouchEvent) => {
    startY.current = e.touches[0].clientY;
    dragging.current = true;
    const el = sheetRef.current;
    if (el) el.style.transition = "none";
  }, []);

  const onDragMove = useCallback((e: React.TouchEvent) => {
    if (!dragging.current) return;
    const dy = e.touches[0].clientY - startY.current;
    if (dy < 0) return; // Only allow drag down
    currentTranslate.current = dy;
    const el = sheetRef.current;
    if (el) el.style.transform = `translateY(${dy}px)`;
  }, []);

  const onDragEnd = useCallback(() => {
    dragging.current = false;
    const el = sheetRef.current;
    if (el) el.style.transition = "transform 0.3s cubic-bezier(0.32, 0.72, 0, 1)";
    if (currentTranslate.current > 120) {
      close();
    } else {
      currentTranslate.current = 0;
      if (el) el.style.transform = "translateY(0)";
    }
  }, [close]);

  return {
    isOpen,
    open,
    close,
    sheetRef,
    onDragStart,
    onDragMove,
    onDragEnd,
  };
}
