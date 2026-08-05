export function containWheelScroll(event: WheelEvent, scroller?: HTMLElement | null): void {
  if (!scroller) return

  const atTop = scroller.scrollTop <= 0
  const atBottom = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 1
  if ((event.deltaY < 0 && atTop) || (event.deltaY > 0 && atBottom)) {
    event.preventDefault()
  }
  event.stopPropagation()
}
