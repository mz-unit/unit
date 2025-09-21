package stores

import (
	"context"
	"crypto/ecdsa"
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
	CreateKey(ctx context.Context) (address string, err error)
	HasKey(ctx context.Context, address string) bool
	SignTx(ctx context.Context, address string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	SignHash(ctx context.Context, address string, hash []byte) ([]byte, error)
}

type LocalKeyStore struct {
	ks             *keystore.KeyStore
	rootDir        string
	unlockDuration time.Duration
	passphrase     string
}

func NewLocalKeyStore(passphrase string, rootDir string) (*LocalKeyStore, error) {
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return nil, err
	}

	ks := keystore.NewKeyStore(rootDir, keystore.StandardScryptN, keystore.StandardScryptP)
	return &LocalKeyStore{ks: ks, passphrase: passphrase, rootDir: rootDir, unlockDuration: 1 * time.Minute}, nil
}

func (l *LocalKeyStore) CreateKey(ctx context.Context) (address string, err error) {
	account, err := l.ks.NewAccount(l.passphrase)
	if err != nil {
		return "", err
	}
	return account.Address.Hex(), nil
}

func (l *LocalKeyStore) HasKey(ctx context.Context, address string) bool {
	return l.ks.HasAddress(common.HexToAddress(address))
}

func (l *LocalKeyStore) SignTx(ctx context.Context, address string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	account, err := l.get(address)
	if err != nil {
		return nil, err
	}

	if err := l.ks.TimedUnlock(account, l.passphrase, l.unlockDuration); err != nil {
		return nil, err
	}
	defer l.ks.Lock(account.Address)

	signedTx, err := l.ks.SignTxWithPassphrase(account, l.passphrase, tx, chainID)
	if err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (l *LocalKeyStore) SignHash(ctx context.Context, address string, hash []byte) ([]byte, error) {
	account, err := l.get(address)
	if err != nil {
		return nil, err
	}

	if err := l.ks.TimedUnlock(account, l.passphrase, l.unlockDuration); err != nil {
		return nil, err
	}
	defer l.ks.Lock(account.Address)

	signedHash, err := l.ks.SignHashWithPassphrase(account, l.passphrase, hash)
	if err != nil {
		return nil, err
	}
	return signedHash, nil
}

func (l *LocalKeyStore) ImportECDSA(privKey *ecdsa.PrivateKey, password string) (string, error) {
	acct, err := l.ks.ImportECDSA(privKey, password)
	if err != nil {
		return "", err
	}

	return acct.Address.Hex(), nil
}

func (l *LocalKeyStore) get(address string) (accounts.Account, error) {
	if !l.ks.HasAddress(common.HexToAddress(address)) {
		return accounts.Account{}, fmt.Errorf("address not found: %s", address)
	}
	account, err := l.ks.Find(accounts.Account{Address: common.HexToAddress(address)})
	if err != nil {
		return accounts.Account{}, err
	}
	return account, nil
}
