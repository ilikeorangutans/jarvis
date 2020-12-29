create table if not exists reminders (id integer, recurring numeric, minute text, hour text, day text, message text, room text, user text, created_at datetime, primary key(id));
create index if not exists reminders_users on reminders (user);
