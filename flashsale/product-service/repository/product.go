package repository

import (
	"database/sql"

	"github.com/google/uuid"
)

type Product struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Stock       int       `json:"stock"`
}

type ProductRepository struct {
	db *sql.DB
}

func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) FindAll() ([]Product, error) {
	rows, err := r.db.Query("SELECT id, name, COALESCE(description, ''), price, stock FROM products")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

func (r *ProductRepository) FindByID(id uuid.UUID) (*Product, error) {
	var p Product
	err := r.db.QueryRow(
		"SELECT id, name, COALESCE(description, ''), price, stock FROM products WHERE id = $1",
		id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepository) UpdateStock(id uuid.UUID, qty int) error {
	_, err := r.db.Exec(
		"UPDATE products SET stock = stock + $1 WHERE id = $2",
		qty, id,
	)
	return err
}

func (r *ProductRepository) DecreaseStock(id uuid.UUID, qty int) error {
	_, err := r.db.Exec(
		"UPDATE products SET stock = stock - $1 WHERE id = $2",
		qty, id,
	)
	return err
}
