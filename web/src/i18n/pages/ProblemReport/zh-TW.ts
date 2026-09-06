import type zhCN from './zh-CN'
export default {
  open: '產生問題報告', title: '本機問題報告', helpTitle: '說明與問題報告',
  help: '操作不符合預期，或是沒有出現錯誤？可以先整理一份問題報告。',
  privacy: '報告只在目前瀏覽器整理，不會自動傳送。請勿填寫密碼或金鑰，匯出前檢查預覽。',
  expected: '你希望發生什麼？', actual: '實際發生了什麼？', optional: '選填，最多 2000 字',
  details: '檢視與刪減診斷資訊', detailsHint: '取消勾選即可移除。缺失表示目前沒有可確認的記錄。',
  preview: '匯出預覽', missing: '缺失', copy: '複製報告', copying: '正在複製…', download: '下載報告',
  copied: '已複製目前預覽。', copyFailed: '複製失敗，請點擊「下載報告」儲存，或從預覽手動複製。',
  downloaded: '已請求瀏覽器儲存報告。', downloadFailed: '無法開始下載，請從預覽手動複製。', close: '關閉',
  snapshot: '這是開啟時的快照；重新開啟可取得最新資訊。關閉後不保留草稿。',
  capturedAt: '快照時間', source: '報告入口', feature: '功能', webVersion: 'Web 版本',
  browser: '瀏覽器類別', clientSystem: '用戶端系統類別', recordState: '工作記錄狀態',
  errorCode: '錯誤碼', httpStatus: 'HTTP 狀態', requestId: '請求 ID', jobId: '工作 ID',
  action: '工作動作', jobStatus: '工作狀態', failureStage: '失敗階段',
} satisfies typeof zhCN
