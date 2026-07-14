-- trace-report.sql — the full post-run analysis of a gospan trace file, one
-- command, six sections. Diagnoses an import run end to end.
--
--   sqlite3 -box <catalog-dir>/traces/gospan-<run>.sqlite < cmd/dev/sql/trace-report.sql
--
-- Reading guide (learned on the first real runs):
--   * READ AGGREGATES FIRST. In a bounded-channel pipeline a downstream
--     bottleneck parks items at whatever upstream seam has buffer room, so a
--     single file's waterfall points wherever it happened to queue; the
--     parallelism section names the true culprit (a pool pinned at its size).
--   * Gaps ARE queue time: stage spans end before the downstream send.
--
-- Durations are adaptively formatted (µs/ms/s/m) — SQLite has no duration
-- type, so each section carries the same CASE+printf formatter inline.

.mode box

.print
.print == stage scoreboard: exact percentiles per span name ==
.print '   (lower = better everywhere; p99 far above p50 = a long tail — a few bad files, not a systemic cost)'
WITH durations AS (
  SELECT n.name, (s.end_ns - s.start_ns) AS dur,
         PERCENT_RANK() OVER (PARTITION BY n.name ORDER BY s.end_ns - s.start_ns) AS pr
  FROM spans s JOIN names n ON n.id = s.name_id
  WHERE s.end_ns IS NOT NULL),
ranked AS (
  SELECT name, COUNT(*) AS n,
         MAX(CASE WHEN pr <= .50 THEN dur END) AS p50,
         MAX(CASE WHEN pr <= .90 THEN dur END) AS p90,
         MAX(CASE WHEN pr <= .99 THEN dur END) AS p99,
         MAX(dur) AS worst
  FROM durations GROUP BY name)
SELECT name, n,
       CASE WHEN p50 >= 6e10 THEN printf('%dm %.1fs', CAST(p50/6e10 AS INT), (p50%6e10)/1e9) WHEN p50 >= 1e9 THEN printf('%.2f s', p50/1e9) WHEN p50 >= 1e6 THEN printf('%.1f ms', p50/1e6) ELSE printf('%.0f µs', p50/1e3) END AS p50,
       CASE WHEN p90 >= 6e10 THEN printf('%dm %.1fs', CAST(p90/6e10 AS INT), (p90%6e10)/1e9) WHEN p90 >= 1e9 THEN printf('%.2f s', p90/1e9) WHEN p90 >= 1e6 THEN printf('%.1f ms', p90/1e6) ELSE printf('%.0f µs', p90/1e3) END AS p90,
       CASE WHEN p99 >= 6e10 THEN printf('%dm %.1fs', CAST(p99/6e10 AS INT), (p99%6e10)/1e9) WHEN p99 >= 1e9 THEN printf('%.2f s', p99/1e9) WHEN p99 >= 1e6 THEN printf('%.1f ms', p99/1e6) ELSE printf('%.0f µs', p99/1e3) END AS p99,
       CASE WHEN worst >= 6e10 THEN printf('%dm %.1fs', CAST(worst/6e10 AS INT), (worst%6e10)/1e9) WHEN worst >= 1e9 THEN printf('%.2f s', worst/1e9) WHEN worst >= 1e6 THEN printf('%.1f ms', worst/1e6) ELSE printf('%.0f µs', worst/1e3) END AS worst
FROM ranked ORDER BY ranked.worst DESC;

.print
.print == effective parallelism: total work / wall clock ==
.print '   (worker POOLS: higher = better, pinned at the pool size = the bottleneck — add workers or move the work;'
.print '    singletons (run/scan/match/write-batch) top out at 1.00x; item/await spans just count concurrent residency)'
WITH wall AS (SELECT MAX(end_ns) - MIN(start_ns) AS run FROM spans WHERE end_ns IS NOT NULL),
work AS (
  SELECT n.name, SUM(s.end_ns - s.start_ns) AS total
  FROM spans s JOIN names n ON n.id = s.name_id
  WHERE s.end_ns IS NOT NULL GROUP BY n.name)
SELECT name,
       CASE WHEN total >= 6e10 THEN printf('%dm %.1fs', CAST(total/6e10 AS INT), (total%6e10)/1e9) WHEN total >= 1e9 THEN printf('%.2f s', total/1e9) WHEN total >= 1e6 THEN printf('%.1f ms', total/1e6) ELSE printf('%.0f µs', total/1e3) END AS total_work,
       printf('%6.2fx', total * 1.0 / wall.run) AS parallelism
FROM work, wall ORDER BY total DESC;

.print
.print == queue map: wait between import stages ==
.print '   (lower = better; a fat avg_wait = backpressure at that seam — blame the stage DOWNSTREAM of the arrow)'
WITH stage AS (
  SELECT s.parent_id AS item, n.name, s.start_ns, s.end_ns
  FROM spans s JOIN names n ON n.id = s.name_id
  WHERE n.name IN ('import.hash','import.match','import.extract','import.thumb')),
gaps AS (
  SELECT 'hash -> match' AS hop, AVG(b.start_ns - a.end_ns) AS avg_gap, MAX(b.start_ns - a.end_ns) AS worst_gap
    FROM stage a JOIN stage b ON b.item = a.item AND a.name = 'import.hash'    AND b.name = 'import.match'
  UNION ALL
  SELECT 'match -> extract', AVG(b.start_ns - a.end_ns), MAX(b.start_ns - a.end_ns)
    FROM stage a JOIN stage b ON b.item = a.item AND a.name = 'import.match'   AND b.name = 'import.extract'
  UNION ALL
  SELECT 'extract -> thumb', AVG(b.start_ns - a.end_ns), MAX(b.start_ns - a.end_ns)
    FROM stage a JOIN stage b ON b.item = a.item AND a.name = 'import.extract' AND b.name = 'import.thumb')
SELECT hop,
       CASE WHEN avg_gap >= 6e10 THEN printf('%dm %.1fs', CAST(avg_gap/6e10 AS INT), (avg_gap%6e10)/1e9) WHEN avg_gap >= 1e9 THEN printf('%.2f s', avg_gap/1e9) WHEN avg_gap >= 1e6 THEN printf('%.1f ms', avg_gap/1e6) ELSE printf('%.0f µs', avg_gap/1e3) END AS avg_wait,
       CASE WHEN worst_gap >= 6e10 THEN printf('%dm %.1fs', CAST(worst_gap/6e10 AS INT), (worst_gap%6e10)/1e9) WHEN worst_gap >= 1e9 THEN printf('%.2f s', worst_gap/1e9) WHEN worst_gap >= 1e6 THEN printf('%.1f ms', worst_gap/1e6) ELSE printf('%.0f µs', worst_gap/1e3) END AS worst
FROM gaps;

.print
.print == batch heartbeat: every WRITE commit ==
.print '   (items near 50 = healthy full batches; small batches ~500ms apart = the lull timer draining a trickle,'
.print '    i.e. upstream is the constraint, not the writer)'
SELECT CAST(ab.value AS INT) AS seq,
       ai.value              AS items,
       CASE WHEN (s.start_ns - t0.first) >= 6e10 THEN printf('%dm %.1fs', CAST((s.start_ns - t0.first)/6e10 AS INT), ((s.start_ns - t0.first)%6e10)/1e9) WHEN (s.start_ns - t0.first) >= 1e9 THEN printf('%.2f s', (s.start_ns - t0.first)/1e9) ELSE printf('%.1f ms', (s.start_ns - t0.first)/1e6) END AS at,
       CASE WHEN (s.end_ns - s.start_ns) >= 1e9 THEN printf('%.2f s', (s.end_ns - s.start_ns)/1e9) WHEN (s.end_ns - s.start_ns) >= 1e6 THEN printf('%.1f ms', (s.end_ns - s.start_ns)/1e6) ELSE printf('%.0f µs', (s.end_ns - s.start_ns)/1e3) END AS took
FROM spans s
JOIN names n  ON n.id = s.name_id AND n.name = 'import.write-batch'
JOIN attrs ab ON ab.span_id = s.id AND ab.key = 'batch_seq'
JOIN attrs ai ON ai.span_id = s.id AND ai.key = 'items',
(SELECT MIN(start_ns) AS first FROM spans) AS t0
ORDER BY seq;

.print
.print == leaderboard: slowest individual spans, with the file responsible ==
.print '   (decode-heavy spans on big files are expected up here; a cheap stage (hash/match) on top = investigate)'
SELECT n.name,
       CASE WHEN (s.end_ns - s.start_ns) >= 6e10 THEN printf('%dm %.1fs', CAST((s.end_ns - s.start_ns)/6e10 AS INT), ((s.end_ns - s.start_ns)%6e10)/1e9) WHEN (s.end_ns - s.start_ns) >= 1e9 THEN printf('%.2f s', (s.end_ns - s.start_ns)/1e9) WHEN (s.end_ns - s.start_ns) >= 1e6 THEN printf('%.1f ms', (s.end_ns - s.start_ns)/1e6) ELSE printf('%.0f µs', (s.end_ns - s.start_ns)/1e3) END AS took,
       COALESCE(own.value, par.value) AS path
FROM spans s
JOIN names n ON n.id = s.name_id
LEFT JOIN attrs own ON own.span_id = s.id        AND own.key = 'path'
LEFT JOIN attrs par ON par.span_id = s.parent_id AND par.key = 'path'
WHERE s.end_ns IS NOT NULL
ORDER BY s.end_ns - s.start_ns DESC LIMIT 15;

.print
.print == trouble: failed/canceled spans, and work left in flight ==
.print '   (want: zero rows in both tables)'
SELECT n.name, CASE s.status WHEN 1 THEN 'error' WHEN 2 THEN 'canceled' END AS status,
       s.error, COALESCE(own.value, par.value) AS path
FROM spans s
JOIN names n ON n.id = s.name_id
LEFT JOIN attrs own ON own.span_id = s.id        AND own.key = 'path'
LEFT JOIN attrs par ON par.span_id = s.parent_id AND par.key = 'path'
WHERE s.status != 0;

SELECT n.name, COUNT(*) AS incomplete_spans
FROM spans s JOIN names n ON n.id = s.name_id
WHERE s.end_ns IS NULL GROUP BY n.name;
