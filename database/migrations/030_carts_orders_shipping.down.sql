
-- Development rollback only.
-- Never run against a database with orders you need to keep.

DROP TABLE IF EXISTS shipment_events;
DROP TABLE IF EXISTS shipments;
DROP TABLE IF EXISTS stock_reservations;
DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS seller_orders;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
