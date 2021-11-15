// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { IERC20 } from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { Lib_OVMCodec } from "../libraries/codec/Lib_OVMCodec.sol";
import { IStateCommitmentChain } from "../L1/rollup/IStateCommitmentChain.sol";

contract MVM_Verifier is Lib_AddressResolver{
    
    enum SETTLEMENT {NOT_ENOUGH_VERIFIER, SAME_ROOT, AGREE, DISAGREE, PASS}
    
    event NewChallenge(uint256 cIndex, uint256 chainID, Lib_OVMCodec.ChainBatchHeader header, uint256 timestamp);
    event Verify1(uint256 cIndex, address verifier);
    event Verify2(uint256 cIndex, address verifier);
    event Finalize(uint256 cIndex, address sender, SETTLEMENT result);
    event Penalize(address sender, uint256 stakeLost);
    event Reward(address target, uint256 amount);
    event Claim(address sender, uint256 amount);
    event Withdraw(address sender, uint256 amount);
    event Stake(address verifier, uint256 amount);
    event SlashSequencer(uint256 chainID, address seq);
    
    /*************
     * Constants *
     *************/
    string constant public CONFIG_OWNER_KEY = "METIS_MANAGER";
    
    //challenge info
    struct Challenge {
       address challenger;
       uint256 chainID;
       uint256 index;
       Lib_OVMCodec.ChainBatchHeader header;
       uint256 timestamp;
       uint256 numQualifiedVerifiers;
       uint256 numVerifiers;
       address[] verifiers;
       bool done;
    }
    
    mapping (address => uint256) public verifier_stakes;
    mapping (uint256 => mapping (address=>bytes)) private challenge_keys;
    mapping (uint256 => mapping (address=>bytes)) private challenge_key_hashes;
    mapping (uint256 => mapping (address=>bytes)) private challenge_hashes;
   
    mapping (address => uint256) public rewards;
    mapping (address => uint8) public absence_strikes;
    mapping (address => uint8) public consensus_strikes;
    
    // only one active challenge for each chain  chainid=>cIndex
    mapping (uint256 => uint256) public chain_under_challenge;
    
    // white list
    mapping (address => bool) public whitelist;
    bool useWhiteList;
    
    address[] public verifiers;
    Challenge[] public challenges;
    
    uint public verifyWindow = 3600 * 24; // 24 hours of window to complete the each verify phase
    uint public activeChallenges;

    address public metis;

    uint256 public minStake;
    bool public seqStaked;
    
    uint256 public numQualifiedVerifiers;
    
    uint FAIL_THRESHOLD = 2;  // 1 time grace
    uint ABSENCE_THRESHOLD = 4;  // 2 times grace

    modifier onlyManager {
        require(
            msg.sender == resolve(CONFIG_OWNER_KEY),
            "MVM_Verifier: Function can only be called by the METIS_MANAGER."
        );
        _;
    }
    
    modifier onlyWhitelisted {
        require(isWhiteListed(msg.sender), "only whitelisted verifiers can call");
        _;
    }
    
    modifier onlyStaked {
        require(isSufficientlyStaked(msg.sender), "insufficient stake");
        _;
    }

    constructor(
      address _addressManager,
      address _metis
    )
      Lib_AddressResolver(_addressManager)
    {
       minStake = 200 ether;  // 200 metis
       metis = _metis;
       useWhiteList = true;
    }
    
    // add stake as a verifier
    function verifierStake(uint256 stake) public onlyWhitelisted{
       require(activeChallenges == 0, "stake is currently prohibited"); //ongoing challenge
       require(stake > 0, "zero stake not allowed");
       require(IERC20(metis).transferFrom(msg.sender, address(this), stake), "transfer metis failed");
       
       uint256 previousBalance = verifier_stakes[msg.sender];
       verifier_stakes[msg.sender] += stake;
       
       require(isSufficientlyStaked(msg.sender), "insufficient stake to qualify as a verifier");
       
       if (previousBalance == 0) {
          numQualifiedVerifiers++;
          verifiers.push(msg.sender);
       }
       
       emit Stake(msg.sender, stake);
    }
    
    // start a new challenge
    // @param chainID chainid
    // @param header chainbatch header
    // @param proposedHash encrypted hash of the correct state
    // @param keyhash hash of the decryption key
    //
    // @dev why do we ask for key and keyhash? because we want verifiers compute the state instead
    // of just copying from other verifiers.
    function newChallenge(uint256 chainID, Lib_OVMCodec.ChainBatchHeader calldata header, bytes calldata proposedHash, bytes calldata keyhash) 
       public onlyWhitelisted onlyStaked {
       
       uint tempIndex = chain_under_challenge[chainID] - 1;
       require(tempIndex == 0 || block.timestamp - challenges[tempIndex].timestamp > verifyWindow * 2, "there is an ongoing challenge");
       if (tempIndex != 0) {
          finalize(tempIndex);
       }
       IStateCommitmentChain stateChain = IStateCommitmentChain(resolve("StateCommitmentChain"));
       
       // while the root is encrypted, the timestamp is available in the extradata field of the header
       require(stateChain.insideFraudProofWindow(header), "the batch is outside of the fraud proof window");
       
       Challenge memory c;
       c.chainID = chainID;
       c.challenger = msg.sender; 
       c.timestamp = block.timestamp;
       c.header = header;
       
       challenges.push(c);
       uint cIndex = challenges.length - 1;
       
       // house keeping
       challenge_hashes[cIndex][msg.sender] = proposedHash;
       challenge_key_hashes[cIndex][msg.sender] = keyhash;
       challenges[cIndex].numVerifiers++;
       
       // this will prevent stake change
       activeChallenges++;
       
       chain_under_challenge[chainID] = cIndex + 1;
       emit NewChallenge(cIndex, chainID, header, block.timestamp);
    }

    // phase 1 of the verify, provide an encrypted hash and the hash of the decryption key
    // @param cIndex index of the challenge
    // @param hash encrypted hash of the correct state (for the index referred in the challenge)
    // @param keyhash hash of the decryption key
    function verify1(uint256 cIndex, bytes calldata hash, bytes calldata keyhash) public onlyWhitelisted onlyStaked{
       require(challenge_hashes[cIndex][msg.sender].length == 0, "verify1 already completed for the sender");
       challenge_hashes[cIndex][msg.sender] = hash;
       challenge_key_hashes[cIndex][msg.sender] = keyhash;
       challenges[cIndex].numVerifiers++;
       emit Verify1(cIndex, msg.sender);
    }
    
    // phase 2 of the verify, provide the actual key to decrypt the hash
    // @param cIndex index of the challenge
    // @param key the decryption key
    function verify2(uint256 cIndex, bytes calldata key) public onlyStaked onlyWhitelisted{
        require(challenges[cIndex].numVerifiers == numQualifiedVerifiers
               || block.timestamp - challenges[cIndex].timestamp > verifyWindow, "phase 2 not ready");
        require(challenge_hashes[cIndex][msg.sender].length > 0, "you didn't participate in phase 1");   
        if (challenge_keys[cIndex][msg.sender].length > 0) {
            finalize(cIndex);
            return;
        }
       
        //verify whether the key matches the keyhash initially provided.
        require(sha256(key) == bytes32(challenge_key_hashes[cIndex][msg.sender]), "key and keyhash don't match");
       
        if (msg.sender == challenges[cIndex].challenger) {
            //decode the root in the header too
            challenges[cIndex].header.batchRoot = bytes32(decrypt(abi.encodePacked(challenges[cIndex].header.batchRoot), key));
        }
        challenge_keys[cIndex][msg.sender] = key;
        challenge_hashes[cIndex][msg.sender] = decrypt(challenge_hashes[cIndex][msg.sender], key);
        challenges[cIndex].verifiers.push(msg.sender);
        emit Verify2(cIndex, msg.sender);
        
        finalize(cIndex);
    }
    
    function finalize(uint256 cIndex) internal {
        
        Challenge storage challenge = challenges[cIndex];
        
        require(challenge.done == false, "challenge is closed");
        
        if (challenge.verifiers.length != challenge.numVerifiers 
           && block.timestamp - challenge.timestamp < verifyWindow * 2) {
           // not ready to finalize. do nothing
           return;
        }
        
        IStateCommitmentChain stateChain = IStateCommitmentChain(resolve("StateCommitmentChain"));
        bytes32 proposedHash = bytes32(challenge_hashes[cIndex][challenge.challenger]);
        
        uint reward = 0;
        
        address[] memory agrees = new address[](challenge.verifiers.length);
        uint numAgrees = 0;
        address[] memory disagrees = new address[](challenge.verifiers.length);
        uint numDisagrees = 0;
    
        for (uint256 i = 0; i < verifiers.length; i++) {
            if (!isSufficientlyStaked(verifiers[i]) || !isWhiteListed(verifiers[i])) {
                // not qualified as a verifier
                continue;
            }
            
            //record the agreement
            if (bytes32(challenge_hashes[cIndex][verifiers[i]]) == proposedHash) {
                //agree with the challenger
                if (absence_strikes[verifiers[i]] > 0) {
                    absence_strikes[verifiers[i]] -= 1; // slowly clear the strike
                }
                agrees[numAgrees] = verifiers[i];
                numAgrees++;
            } else if (challenge_keys[cIndex][verifiers[i]].length == 0) {
                //absent
                absence_strikes[verifiers[i]] += 2;
                if (absence_strikes[verifiers[i]] > ABSENCE_THRESHOLD) {
                    reward += penalize(verifiers[i]);
                }
            } else {
                //disagree with the challenger
                if (absence_strikes[verifiers[i]] > 0) {
                    absence_strikes[verifiers[i]] -= 1; // slowly clear the strike
                }
                disagrees[numDisagrees] = verifiers[i];
                numDisagrees++;
            }
        }
        
        if (Lib_OVMCodec.hashBatchHeader(challenge.header) != 
                stateChain.batches().getByChainId(challenge.chainID, challenge.header.batchIndex)) {
            // wrong header, penalize the challenger
            reward += penalize(challenge.challenger);
            
            // reward the disagrees. but no penalty on agrees because the input
            // is garbage.
            distributeReward(reward, disagrees, challenge.verifiers.length - 1);
            emit Finalize(cIndex, msg.sender, SETTLEMENT.DISAGREE);
            
        } else if (challenge.verifiers.length < numQualifiedVerifiers * 75 / 100) {
            // the absent verifiers get a absense strike. no other penalties. already done
            emit Finalize(cIndex, msg.sender, SETTLEMENT.NOT_ENOUGH_VERIFIER);
        }
        else if (proposedHash != challenge.header.batchRoot) {
            if (numAgrees <= numDisagrees) {
               // no consensus, challenge failed. 
               for (uint i = 0; i < numAgrees; i++) {
                    consensus_strikes[agrees[i]] += 2;
                    if (consensus_strikes[agrees[i]] > FAIL_THRESHOLD) {
                        reward += penalize(agrees[i]);
                    }
               }
               distributeReward(reward, disagrees, disagrees.length);
               emit Finalize(cIndex, msg.sender, SETTLEMENT.DISAGREE);
            } else {
               // reached agreement. delete the batch root and slash the sequencer if the header is still valid
               if(stateChain.insideFraudProofWindow(challenge.header)) {
                    // this header needs to be within the window
                    stateChain.deleteStateBatchByChainId(challenge.chainID, challenge.header);
               
                    reward += 200 ether;
               
                    for (uint i = 0; i < numDisagrees; i++) {
                        consensus_strikes[disagrees[i]] += 2;
                        if (consensus_strikes[disagrees[i]] > FAIL_THRESHOLD) {
                            reward += penalize(disagrees[i]);
                        }
                    }
                    distributeReward(reward, agrees, agrees.length);
                    emit Finalize(cIndex, msg.sender, SETTLEMENT.AGREE);
                } else {
                    //not in the window anymore. let it pass... no penalty
                    emit Finalize(cIndex, msg.sender, SETTLEMENT.PASS);
                }
            }
        } else {
            //wasteful challenge, add consensus_strikes
            for (uint i = 0; i < numAgrees; i++) {
                consensus_strikes[agrees[i]] += 2;
                if (consensus_strikes[agrees[i]] > FAIL_THRESHOLD) {
                    reward += penalize(agrees[i]);
                }
            }
            distributeReward(reward, disagrees, disagrees.length);
            emit Finalize(cIndex, msg.sender, SETTLEMENT.SAME_ROOT);
        }
        
        challenge.done = true;
        activeChallenges--;
        chain_under_challenge[challenge.chainID] = 0;
    }
    
    function depositSeqStake() public onlyManager {
        require(seqStaked == false, "already staked");
        seqStaked = true;
        require(IERC20(metis).transferFrom(msg.sender, address(this), 200 ether), "transfer metis failed");
        emit Stake(msg.sender, 200 ether);
    }
    
    function withdrawSeqStake(address to) public onlyManager {
        require(seqStaked == true, "no stake");
        seqStaked = false;
        require(IERC20(metis).transfer(to, 200 ether), "transfer metis failed");
        emit Withdraw(msg.sender, 200 ether);
    }
    
    function claim() public {
       require(rewards[msg.sender] > 0, "no reward to claim");
       uint256 amount = rewards[msg.sender];
       rewards[msg.sender] = 0;

       require(IERC20(metis).transfer(msg.sender, amount), "token transfer failed");
       
       emit Claim(msg.sender, amount);
    }

    function withdraw(uint256 amount) public {
       require(activeChallenges == 0, "withdraw is currently prohibited"); //ongoing challenge
       
       uint256 balance = verifier_stakes[msg.sender];
       require(balance >= amount, "insufficient stake to withdraw");
       
       if (balance - amount < minStake && balance >= minStake) {
          numQualifiedVerifiers--;
       }
       verifier_stakes[msg.sender] -= amount;
       
       require(IERC20(metis).transfer(msg.sender, amount), "token transfer failed");
    }
    
    function setMinStake(
        uint256 _minStake
    )
        public
        onlyManager
    {
        minStake = _minStake;
        uint num = 0;
        for (uint i = 0; i < verifiers.length; ++i) {
          if (verifier_stakes[verifiers[i]] >= minStake) {
             num++;
          }
        }
        numQualifiedVerifiers = num;
    }
    
    // helper
    function isWhiteListed(address verifier) view public returns(bool){
        return !useWhiteList || whitelist[verifier];
    }
    function isSufficientlyStaked (address target) view public returns(bool) {
       return (verifier_stakes[target] >= minStake);
    }
    
    // set the length of the time windows for each verification phase
    function setVerifyWindow (uint256 window) onlyManager public {
        verifyWindow = window;
    }
    
    // set the length of the time windows for each verification phase
    function resetNumVerifiers (uint256 num) onlyManager public {
        numQualifiedVerifiers = num;
    }
    
    // add the verifier to the whitelist
    function setWhiteList(address verifier, bool allowed) public onlyManager {
        whitelist[verifier] = allowed;
        useWhiteList = true;
    }
    
    // allow everyone to be the verifier
    function disableWhiteList() public onlyManager {
        useWhiteList = false;
    }
    
    function setThreshold(uint absence_threshold, uint fail_threshold) public onlyManager {
        ABSENCE_THRESHOLD = absence_threshold;
        FAIL_THRESHOLD = fail_threshold;
    }
    
    //helper fucntion to encrypt data
    function encrypt(bytes calldata data, bytes calldata key) pure public returns (bytes memory) {
      bytes memory encryptedData = data;
      uint j = 0;
      
      for (uint i = 0; i < encryptedData.length; i++) {
          if (j == key.length) {
             j = 0;
          } 
          encryptedData[i] = encryptByte(encryptedData[i], uint8(key[j]));
          j++;
      }

      return encryptedData;
    }

    function encryptByte(bytes1 b, uint8 k) pure internal returns (bytes1) {
      uint16 temp16 = uint16(uint8(b));
      temp16 += k;
      
      if (temp16 > 255) {
         temp16 -= 256;
      } 
      return bytes1(uint8(temp16));
    }
    
    // helper fucntion to decrypt the data
    function decrypt(bytes memory data, bytes memory key) pure internal returns (bytes memory) {
      bytes memory decryptedData = data;
      uint j = 0;
      
      for (uint i = 0; i < decryptedData.length; i++) {
          if (j == key.length) {
             j = 0;
          }
          
          decryptedData[i] = decryptByte(decryptedData[i], uint8(key[j]));
          
          j++;
      }

      return decryptedData;
    }

    function decryptByte(bytes1 b, uint8 k) pure internal returns (bytes1) {
      uint16 temp16 = uint16(uint8(b));
      if (temp16 > k) {
         temp16 -= k;
      } else {
         temp16 = 256 - k;
      }

      return bytes1(uint8(temp16));
    }
    
    // calculate the rewards
    function distributeReward(uint256 amount, address[] memory list, uint num) internal {
        uint reward = amount / num;
        if (reward == 0) {
            return;
        }
        uint total = 0;
        for (uint i; i < list.length; i++) {
            if (isSufficientlyStaked(list[i])) {
               rewards[list[i]] += reward;
               total += reward;
               emit Reward(list[i], reward);
            }
        }
        
        if (total < amount) {
            if (isSufficientlyStaked(list[0])) {
                rewards[list[0]] += total - amount;
                emit Reward(list[0], total - amount);
            } else {
                rewards[list[1]] += total - amount;
                emit Reward(list[1], total - amount);
            }
        }
    }
    
    // slash the verifier stake
    function penalize(address target) internal returns(uint256) {
        uint256 stake = verifier_stakes[target];
        verifier_stakes[target] = 0;
        numQualifiedVerifiers--;
        emit Penalize(target, stake);
        
        return stake;
    }
    
}
