package stores

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type KeyStore interface {
	CreateAccount(ctx context.Context) (address string, err error)
	GetAccount(ctx context.Context, address string) (accounts.Account, error)
	SignTx(ctx context.Context, address string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	SignHash(ctx context.Context, address string, hash []byte) ([]byte, error)
}

type LocalKeyStore struct {
	ks             *keystore.KeyStore
	rootDir        string
	unlockDuration time.Duration
	passphrase     string
}

func NewLocalKeyStore(passphrase string, rootDir string, unlockDuration time.Duration) (*LocalKeyStore, error) {
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return nil, err
	}

	ks := keystore.NewKeyStore(rootDir, keystore.StandardScryptN, keystore.StandardScryptP)
	return &LocalKeyStore{ks: ks, passphrase: passphrase, rootDir: rootDir, unlockDuration: unlockDuration}, nil
}

func (l *LocalKeyStore) CreateAccount(ctx context.Context) (address string, err error) {
	account, err := l.ks.NewAccount(l.passphrase)
	if err != nil {
		return "", err
	}
	return account.Address.Hex(), nil
}

func (l *LocalKeyStore) GetAccount(ctx context.Context, address string) (accounts.Account, error) {
	if !l.ks.HasAddress(common.HexToAddress(address)) {
		return accounts.Account{}, fmt.Errorf("address not found: %s", address)
	}
	account, err := l.ks.Find(accounts.Account{Address: common.HexToAddress(address)})
	if err != nil {
		return accounts.Account{}, err
	}
	return account, nil
}

func (l *LocalKeyStore) SignTx(ctx context.Context, address string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	account, err := l.GetAccount(ctx, address)
	if err != nil {
		return nil, err
	}

	if err := l.ks.TimedUnlock(account, l.passphrase, l.unlockDuration); err != nil {
		return nil, fmt.Errorf("error unlocking account: %w", err)
	}
	defer l.ks.Lock(account.Address)

	signedTx, err := l.ks.SignTxWithPassphrase(account, l.passphrase, tx, chainID)
	if err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (l *LocalKeyStore) SignHash(ctx context.Context, address string, hash []byte) ([]byte, error) {
	account, err := l.GetAccount(ctx, address)
	if err != nil {
		return nil, err
	}

	if err := l.ks.TimedUnlock(account, l.passphrase, l.unlockDuration); err != nil {
		return nil, fmt.Errorf("error unlocking account: %w", err)
	}
	defer l.ks.Lock(account.Address)

	signedHash, err := l.ks.SignHashWithPassphrase(account, l.passphrase, hash)
	if err != nil {
		return nil, err
	}
	return signedHash, nil
}
