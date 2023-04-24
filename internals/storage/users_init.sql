DROP TABLE IF EXISTS users;
CREATE TABLE users (
    id        serial primary key,
    user_id   bytea       not null unique check (length(user_id) = 16),
    username  varchar(32) not null unique,
    password  bytea       not null
);
SELECT nextval(pg_get_serial_sequence('users', 'id'));
