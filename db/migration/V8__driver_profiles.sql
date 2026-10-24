-- Optional driver profile fields remain NULL until an operator supplies them.
-- Shift times are local wall-clock times and are stored separately for dispatch.

alter table drivers
    add column contact_phone text,
    add column license_class text,
    add column license_expiry date,
    add column shift_start time,
    add column shift_end time,
    add column availability text,
    add constraint drivers_shift_complete check (
        (shift_start is null and shift_end is null) or
        (shift_start is not null and shift_end is not null)
    ),
    add constraint drivers_shift_clock_values check (
        (shift_start is null or (shift_start < time '24:00' and extract(second from shift_start) = 0)) and
        (shift_end is null or (shift_end < time '24:00' and extract(second from shift_end) = 0))
    ),
    add constraint drivers_availability_known check (
        availability is null or availability in ('available', 'unavailable')
    );
