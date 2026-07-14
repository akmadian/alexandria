-- trace-asset.sql — the biography of ONE asset: its waterfall, offsets relative
-- to its own start. Gaps between rows are time spent queuing (backpressure) —
-- remember: WHERE an item queues is lottery; use trace-report's parallelism
-- section to name the bottleneck, this script to narrate one victim.
--
-- Default subject: the slowest asset in the file. To pick a specific file,
-- swap the ORDER BY line for:   AND s.id IN (SELECT span_id FROM attrs
--                                WHERE key='path' AND value LIKE '%IMG_1234%')
--
--   sqlite3 -box <catalog-dir>/traces/gospan-<run>.sqlite < cmd/dev/sql/trace-asset.sql

.mode box

WITH subject AS (
  SELECT s.id, s.start_ns,
         (SELECT value FROM attrs WHERE span_id = s.id AND key = 'path') AS path
  FROM spans s JOIN names n ON n.id = s.name_id
  WHERE n.name = 'import.asset' AND s.end_ns IS NOT NULL
  ORDER BY s.end_ns - s.start_ns DESC
  LIMIT 1)
SELECT n.name,
       CASE WHEN (s.start_ns - subject.start_ns) >= 6e10 THEN printf('%dm %.1fs', CAST((s.start_ns - subject.start_ns)/6e10 AS INT), ((s.start_ns - subject.start_ns)%6e10)/1e9) WHEN (s.start_ns - subject.start_ns) >= 1e9 THEN printf('%.2f s', (s.start_ns - subject.start_ns)/1e9) WHEN (s.start_ns - subject.start_ns) >= 1e6 THEN printf('%.1f ms', (s.start_ns - subject.start_ns)/1e6) ELSE printf('%.0f µs', (s.start_ns - subject.start_ns)/1e3) END AS at,
       CASE WHEN (s.end_ns - s.start_ns) >= 6e10 THEN printf('%dm %.1fs', CAST((s.end_ns - s.start_ns)/6e10 AS INT), ((s.end_ns - s.start_ns)%6e10)/1e9) WHEN (s.end_ns - s.start_ns) >= 1e9 THEN printf('%.2f s', (s.end_ns - s.start_ns)/1e9) WHEN (s.end_ns - s.start_ns) >= 1e6 THEN printf('%.1f ms', (s.end_ns - s.start_ns)/1e6) ELSE printf('%.0f µs', (s.end_ns - s.start_ns)/1e3) END AS took,
       CASE s.status WHEN 1 THEN 'ERROR: ' || s.error WHEN 2 THEN 'canceled' ELSE '' END AS note,
       subject.path
FROM spans s JOIN names n ON n.id = s.name_id, subject
WHERE s.id = subject.id OR s.parent_id = subject.id
ORDER BY s.start_ns;
