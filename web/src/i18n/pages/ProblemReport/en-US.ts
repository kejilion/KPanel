import type zhCN from './zh-CN'
export default {
  open: 'Create problem report', title: 'Local problem report', helpTitle: 'Help and problem reports',
  help: 'Unexpected behavior, even without an error? Prepare a problem report here.',
  privacy: 'Prepared only in this browser, with no automatic sending. Do not enter passwords or keys. Review before exporting.',
  expected: 'What did you expect?', actual: 'What actually happened?', optional: 'Optional, up to 2000 characters',
  details: 'Review and remove diagnostic details', detailsHint: 'Uncheck to remove a field. Missing means no confirmed record is available.',
  preview: 'Export preview', missing: 'Missing', copy: 'Copy report', copying: 'Copying…', download: 'Download report',
  copied: 'Current preview copied.', copyFailed: 'Copy failed. Click “Download report” to save, or copy manually from the preview.',
  downloaded: 'Browser asked to save the report.', downloadFailed: 'Download could not start. Copy manually from the preview.', close: 'Close',
  snapshot: 'Snapshot taken when opened. Reopen for fresh information. Closing discards the draft.',
  capturedAt: 'Snapshot time', source: 'Report entry', feature: 'Feature', webVersion: 'Web version',
  browser: 'Browser category', clientSystem: 'Client system category', recordState: 'Job record state',
  errorCode: 'Error code', httpStatus: 'HTTP status', requestId: 'Request ID', jobId: 'Job ID',
  action: 'Job action', jobStatus: 'Job status', failureStage: 'Failed stage',
} satisfies typeof zhCN
