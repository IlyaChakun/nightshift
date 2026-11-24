-- name: GetSetting :one
SELECT * FROM book.settings WHERE tenant = @tenant AND key = @key;

-- name: PutSetting :one
INSERT INTO book.settings (tenant, key, value, updated_by)
VALUES (@tenant, @key, @value, @updated_by)
ON CONFLICT (tenant, key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = now()
RETURNING *;
