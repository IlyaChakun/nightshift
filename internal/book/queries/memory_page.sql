-- name: GetMemoryPage :one
SELECT * FROM book.memory_pages WHERE tenant = @tenant;

-- name: CreateMemoryPage :execrows
INSERT INTO book.memory_pages (tenant, revision, entries)
VALUES (@tenant, 1, @entries)
ON CONFLICT (tenant) DO NOTHING;

-- name: UpdateMemoryPage :execrows
UPDATE book.memory_pages
SET revision = revision + 1, entries = @entries, updated_at = now()
WHERE tenant = @tenant AND revision = @revision;
