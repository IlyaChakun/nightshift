# Nightly export

## Problem

Reports are slow to produce.

## Goals

- Make the export faster and better.

## Non-goals

- A new UI.

## Approach

Send export jobs to a new message broker and read them from a worker.

## Acceptance Criteria

- Given a report, When the nightly job runs, Then the export file exists.
