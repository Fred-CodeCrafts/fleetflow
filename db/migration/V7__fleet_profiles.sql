-- Optional profile fields remain NULL until an operator supplies them. NULL is
-- displayed as unknown; no missing value is inferred from the vehicle type or
-- a driver's name.

alter table vehicles
    add column registration text,
    add column make text,
    add column model text,
    add column year integer,
    add column color text,
    add column body_type text,
    add column fuel_type text,
    add column capacity numeric,
    add column maintenance_history text,
    add constraint vehicles_year_valid check (year is null or year >= 1886),
    add constraint vehicles_capacity_positive check (capacity is null or capacity > 0);
