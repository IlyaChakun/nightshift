-- +goose Up
-- Chat messages find their task by thread (phase 1, item 7).
CREATE INDEX tasks_thread ON book.tasks (tenant, thread_ref) WHERE thread_ref IS NOT NULL;
