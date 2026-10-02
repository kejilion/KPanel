/**
 * What the shared host picker shows for one host. Each page decides the label
 * (already translated), what choosing the host does, and a status tone; the
 * picker only renders them, so file, gallery and monitoring pages look alike.
 */
export type HostSwitcherAction = 'select' | 'manage' | 'open'
export type HostSwitcherTone = 'online' | 'warning' | 'offline' | 'neutral'

export interface HostSwitcherStatus {
  label: string
  /** select: switch here; manage: needs attention elsewhere; open: opens another Panel. Defaults to select. */
  action?: HostSwitcherAction
  /** Drawn as a small dot before the label; the label always carries the meaning. */
  tone?: HostSwitcherTone
}
