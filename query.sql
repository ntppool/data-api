-- name: GetZoneStatsData :many
SELECT zc.date, z.name, zc.ip_version, count_active, count_registered, netspeed_active
FROM zone_server_counts zc USE INDEX (date_idx)
  INNER JOIN zones z
    ON(zc.zone_id=z.id)
  WHERE date IN (SELECT max(date) from zone_server_counts)
ORDER BY name;
