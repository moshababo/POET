package poet

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
)

// // This type will provide the inteface to the Prover. It implements the
// // io.ReadWriter interface, which will allow it to sit behind an RPC Server
// // or be linked directly to a verifier.
// // CurrentState is used to
type Prover struct {
	CreateNIPChallenge bool
	CurrentState       State
	rootHash           []byte
	challengeProof     []byte
	// other types based on implementation. Eg leveldb client & DAG
}

func NewProver(CreateChallenge bool) *Prover {
	p := new(Prover)
	p.CreateNIPChallenge = CreateChallenge
	return p
}

// // Satifying io.ReadWriter interface. In Start State it returns Proof from
// // Commitment. In WaitingChalleng State, it returns Challenge Proof. Both
// // commitment and challenge are encoded as a byte slice (b). To retrieve
// // the proof, the verifier calls Read.
func (p *Prover) Write(b []byte) (n int, err error) {
	if p.CurrentState == Start {
		var commitParam CommitProofParam
		commitParam.commitment = b
		err = p.CalcCommitProof(commitParam)
		p.CurrentState = Commited
	} else if p.CurrentState == WaitingChallenge {
		err = p.CalcChallengeProof(b)
		p.CurrentState = ProofDone
	} else {
		return 0, errors.New("Prover in Wrong State for Write")
	}
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (p *Prover) Read(b []byte) (n int, err error) {
	// TODO: Check size of b. Read only supposed to send len(b) bytes. If
	// not big enough, need to return error
	if p.CurrentState == Commited {
		b, err = p.SendCommitProof()
		p.CurrentState = WaitingChallenge
	} else if p.CurrentState == ProofDone {
		b, err = p.SendChallengeProof()
		p.CurrentState = Start // For now, this just loops back.
		// TODO: What is the logic to change state? May have other functions to
		// reset prover to Start state (clear DAG and ready for new statement)
	} else {
		return 0, errors.New("Prover in Wrong State for Read")
	}
	return 0, nil
}

// WriteToFile write the labels at height m to file
func (p *Prover) WriteToFile(data []byte) error {
	file, err := os.Create(filepath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	// write to file
	fmt.Fprintln(w, data)
	return w.Flush()
}

// ReadFile
func (p *Prover) ReadLabelFile(offset int) ([]byte, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	i := 0
	var data []byte
	for scanner.Scan() {
		if i != offset {
			i++
			continue
		}
		data = scanner.Bytes()
		break
	}
	return data, nil
}

// GetParents get parents of a node
func (p *Prover) GetParents(node *BinaryID) ([]*BinaryID, error) {
	var parents []*BinaryID

	bitlist := node.BitList()
	length := len(bitlist)

	if length == n {
		for i := 1; i < (length + 1); i++ {
			if bitlist[i-1] == 1 {
				data := append(bitlist[:i-1], 0)
				id, _ := NewBinaryID(uint(i), BitsToInt(data))
				parents = append(parents, id)
			}
		}
	} else {
		data0 := append(bitlist, 0)
		id0, _ := NewBinaryID(uint(length+1), BitsToInt(data0))
		parents = append(parents, id0)

		data1 := append(bitlist, 1)
		id1, _ := NewBinaryID(uint(length+1), BitsToInt(data1))
		parents = append(parents, id1)
	}

	// sort the parent ids
	sort.Slice(parents, func(a, b int) bool {
		return parents[a].GreaterThan(parents[b])
	})

	// get the byte values of the parents
	return parents, nil
}

// ComputeLabel of a node id
func (p *Prover) ComputeLabel(commitment []byte, node *BinaryID, hash HashFunc) []byte {
	parents, _ := p.GetParents(node)

	// should contain the concatenated byte array
	// of parent labels
	var parentLabels []byte
	// maps the string encoding of a node id
	// to its label bytes
	var computed map[string][]byte

	// Loop through the parents and try to calculate their labels
	// if doesn't exist in computed
	for i := 0; i < len(parents); i++ {
		// convert the byte array to a string representation
		str := fmt.Sprintf("%s", parents[i].Encode())
		// check if the label exists in computed
		if _, ok := computed[str]; ok {
			parentLabels = append(parentLabels, computed[str]...)
		} else {
			// compute the label
			label := p.ComputeLabel(commitment, node, hash)
			// store it in computed
			computed[str] = label
			parentLabels = append(parentLabels, label...)
		}
	}

	result := hash.HashVals(commitment, node.Val, parentLabels)
	return result
}

// ConstructDag create dag
// returns the root hash of the dag as []byte
func (p *Prover) ConstructDag(commitment []byte, hash HashFunc) ([]byte, error) {
	// was told no need to use a graph anymore
	// can just compute the edges using an algorithm
	var l0, l1 []byte

	// for height from 0 to m
	for height := 0; height < (m + 1); height++ {
		// compute number of nodes for each sub tree
		numberOfNodes := int(math.Pow(float64(2), float64(height)))

		/**
		* Improvement: Can use a single loop and write offsets file
		* File offsets seems not quite easy to do cos of unknown
		* buffer length
		 */

		// left sub tree
		// perform left sub tree calculation
		for level := 0; level < numberOfNodes; level++ {
			leftId, _ := NewBinaryID(uint(height), level)
			leftId.AddBit(0)
			leftLabel := p.ComputeLabel(commitment, leftId, hash)
			if height == 1 {
				l0 = leftLabel
			}
			p.WriteToFile(leftLabel)
		}

		// right sub tree
		// pefrom right sub tree calculation
		for level := 0; level < numberOfNodes; level++ {
			rightId, _ := NewBinaryID(uint(height), level)
			rightId.AddBit(1)
			rightLabel := p.ComputeLabel(commitment, rightId, hash)
			if height == 1 {
				l1 = rightLabel
			}
			p.WriteToFile(rightLabel)
		}
	}

	rootHash := hash.HashVals(commitment, l0, l1)
	return rootHash, nil
}

type CommitProofParam struct {
	commitment []byte
	hash       HashFunc
}

// CalcCommitProof calculates the proof of seqeuntial work
func (p *Prover) CalcCommitProof(param CommitProofParam) error {
	var hashFunction HashFunc

	hashFunction = param.hash
	if hashFunction == nil {
		hashFunction = NewSHA256()
	}
	//
	phi, _ := p.ConstructDag(param.commitment, hashFunction)
	p.rootHash = phi
	return nil
}

// SendCommitProof send the phi (root Hash) to the verifier
func (p *Prover) SendCommitProof() (b []byte, err error) {
	return p.rootHash, nil
}

// CalcNIPCommitProof proof created by computing openH for the challenge
func (p *Prover) CalcNIPCommitProof(commitment []byte, phi []byte) error {
	var proof []byte
	proof = make([]byte, 32)

	hash := NewSHA256()

	for i := 0; i < t; i++ {
		scParam := make([]byte, binary.MaxVarintLen64)
		binary.BigEndian.PutUint64(scParam, uint64(i))
		proof = append(proof, hash.HashVals(phi, commitment, scParam)...)
	}
	p.challengeProof = proof
	return nil
}

// Siblings returns the list of siblings along the path to the root
//
// Takes in an instance of class BinaryString and returns a list of the
// siblings of the nodes of the path to to root of a binary tree. Also
// returns the node itself, so there are N+1 items in the list for a
// tree with length N.
//
func (p *Prover) Siblings(node *BinaryID) ([]*BinaryID, error) {

	var siblings []*BinaryID
	siblings = append(siblings, node)

	uintValue, _ := binary.Uvarint(node.Val)
	newBinaryID, _ := NewBinaryID(uint(uintValue), node.Length)

	for i := 0; i < node.Length; i++ {
		newBinaryID.FlipBit(0)
		siblings = append(siblings, newBinaryID)
		newBinaryID.TruncateLastBit()
	}

	return siblings, nil
}

// CalcChallengeProof
func (p *Prover) CalcChallengeProof(gamma []byte) error {
	// tuple_lst = []
	// # First get the list
	// for gamma_i in gamma:
	//     label_gamma_i = G.node[gamma_i]['label']
	//     label_gamma_i_siblings = {}
	//     for sib in path_siblings(gamma_i):
	//         label_gamma_i_siblings[sib] = G.node[sib]['label']
	//     tuple_lst += [(label_gamma_i, label_gamma_i_siblings)]
	// return tuple_lst

	var proof []byte

	gamma_BinID := NewBinaryIDBytes(gamma)
	siblings, err := p.Siblings(gamma_BinID)
	if err != nil {
		return nil
	}

	label_gamma_index := (int(math.Pow(float64(2), float64(gamma_BinID.Length+1))) - 1) + len(siblings)

	label_gamma, err := p.ReadLabelFile(label_gamma_index)
	if err != nil {
		return err
	}

	proof = append(proof, label_gamma...)

	for i := 0; i < len(siblings); i++ {
		nodeID := siblings[i]
		nodeSiblings, err := p.Siblings(nodeID)
		if err != nil {
			return err
		}

		sibling_index := (int(math.Pow(float64(2), float64(nodeID.Length+1))) - 1) + len(nodeSiblings)
		label, err := p.ReadLabelFile(sibling_index)
		if err != nil {
			return err
		}
		proof = append(proof, label...)
	}

	p.challengeProof = proof
	return nil
}

// SendChallengeProof
func (p *Prover) SendChallengeProof() (b []byte, err error) {
	return b, nil
}

type State int

const (
	Start State = iota
	Commited
	WaitingChallenge
	ProofDone
)