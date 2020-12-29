alter table reminders add column entry_id integer;
create index if not exists reminders_entry_id on reminders (entry_id);
